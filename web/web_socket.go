package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"smart-home/event"
	"smart-home/internal"
	"sync"
	"time"

	"github.com/go-chi/jwtauth/v5"
	"github.com/gorilla/websocket"
)

type WebSocketServer struct {
	clients  map[*websocket.Conn]bool
	upgrader websocket.Upgrader
	lock     *sync.Mutex
}

func NewWebSocketServer() *WebSocketServer {
	var clients = make(map[*websocket.Conn]bool)

	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true }, // Allow all connections
	}

	return &WebSocketServer{
		clients:  clients,
		upgrader: upgrader,
		lock:     &sync.Mutex{},
	}
}

// wsCloseTokenExpired is the close code sent when the connection's JWT
// expires (4000-4999 is the application-defined range).
const wsCloseTokenExpired = 4001

func (s *WebSocketServer) handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer ws.Close()

	// The HTTP server's Read/WriteTimeout deadlines carry over to the hijacked
	// connection; clear them so long-lived sockets aren't cut off.
	_ = ws.NetConn().SetDeadline(time.Time{})

	// Close the socket when its JWT expires; the client must reconnect with a
	// fresh token. The authenticator guarantees a valid token with exp here.
	if token, _, err := jwtauth.FromContext(r.Context()); err == nil && token != nil {
		if exp, ok := token.Expiration(); ok {
			timer := time.AfterFunc(time.Until(exp), func() {
				msg := websocket.FormatCloseMessage(wsCloseTokenExpired, "token expired")
				_ = ws.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
				_ = ws.Close()
			})
			defer timer.Stop()
		}
	}

	s.clients[ws] = true

	defer func() {
		delete(s.clients, ws)
		ws.Close()
	}()

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			fmt.Println("read error:", err)
			delete(s.clients, ws)
			break
		}
		slog.Info("Received WS message: " + string(msg))
	}
}

// WsSeriesEvent is the `series` channel: every series of a device, sent when one is created
// or its label/hidden flag changes.
type WsSeriesEvent struct {
	DeviceId uint             `json:"deviceId"`
	Series   []SeriesResponse `json:"series"`
}

// Subscribe must run after the event handlers subscribed: the `sensor` events read the live
// series values (DeviceState.Values) the handlers just set.
func (s *WebSocketServer) Subscribe(dispatcher *event.Dispatcher, states *internal.DeviceStateManager, series SeriesSource) {
	dispatcher.Subscribe(
		"SeriesChanged",
		func(raw any) {
			e := raw.(*event.SeriesChanged)
			s.Send("series", WsSeriesEvent{DeviceId: e.DeviceId, Series: newSeriesList(series.DeviceSeries(e.DeviceId))})
		},
	)

	dispatcher.Subscribe(
		"Power",
		func(raw any) {
			e := raw.(event.MqttEvent)
			p := e.Payload.(*event.Power)

			if p == nil {
				slog.Error("No power state in the event")
				return
			}

			deviceState := states.GetByTopic(e.Topic)
			if deviceState == nil { // in-flight message of a deleted or renamed device
				return
			}

			on := bool(*p)
			wsEvent := WsStateEvent{
				ID: deviceState.Device.ID,
				On: &on,
			}
			s.Send("state", wsEvent)
		},
	)

	dispatcher.Subscribe(
		"Status10", // ToDo: map event name somehow
		func(raw any) {
			e := raw.(event.MqttEvent)
			p := e.Payload.(*event.Status10)
			deviceState := states.GetByTopic(e.Topic)
			if deviceState == nil { // in-flight message of a deleted or renamed device
				return
			}

			if !internal.HasSensorData(deviceState.Device.SensorType, p.StatusSNS) {
				return
			}
			wsEvent := NewSensorFromEvent(p.StatusSNS, deviceState.Device, maps.Clone(deviceState.Values))
			s.Send("sensor", wsEvent)
		},
	)

	dispatcher.Subscribe(
		"SensorEvent",
		func(raw any) {
			e := raw.(event.MqttEvent)
			p := e.Payload.(*event.SensorEvent)
			deviceState := states.GetByTopic(e.Topic)
			if deviceState == nil { // in-flight message of a deleted or renamed device
				return
			}

			// IR remotes and unrecognised devices: their live values (e.g. only the chip temperature)
			if !internal.HasSensorData(deviceState.Device.SensorType, p) &&
				!(deviceState.Device.AvailabilityFromLwt() && len(deviceState.Values) > 0) {
				return
			}
			wsEvent := NewSensorFromEvent(p, deviceState.Device, maps.Clone(deviceState.Values))
			s.Send("sensor", wsEvent)
		},
	)

	// IR remotes and unrecognised devices get their availability from LWT, which emits no
	// Power or InternalOfflineEvent, so push their online/offline transitions explicitly.
	// Sensor devices without a relay get no POWER reply either, so push their online
	// transition too (their offline one goes out via InternalOfflineEvent).
	dispatcher.Subscribe(
		"DeviceAvailability",
		func(raw any) {
			e := raw.(*event.DeviceAvailability)
			deviceState := states.GetById(e.DeviceId)
			if deviceState == nil {
				return
			}
			if !deviceState.Device.AvailabilityFromLwt() && (!e.Online || deviceState.CanToggle()) {
				return
			}

			wsEvent := WsStateEvent{ID: e.DeviceId}
			if e.Online {
				wsEvent.On = deviceState.On // true, or for relays the last POWER (nil until it arrives)
			}
			s.Send("state", wsEvent)
		},
	)

	dispatcher.Subscribe(
		"InternalOfflineEvent",
		func(raw any) {
			e := raw.(*event.InternalOfflineEvent)
			wsEvent := WsStateEvent{
				ID: e.DeviceId,
				On: nil,
			}
			s.Send("state", wsEvent)
		},
	)
}

type WsMessage struct {
	Channel string `json:"channel"`
	Body    any    `json:"body"`
}

func (s *WebSocketServer) Send(channel string, body any) {
	message := WsMessage{
		Channel: channel,
		Body:    body,
	}

	payload, err := json.Marshal(message)
	if err != nil {
		slog.Error("serialization error", "err", err)
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	for client := range s.clients {
		if err := client.WriteMessage(websocket.TextMessage, payload); err != nil {
			fmt.Println("broadcast error:", err)
			client.Close()
			delete(s.clients, client)
		}
	}
}
