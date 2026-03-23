package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"smart-home/model"
	"sync"

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

func (s *WebSocketServer) handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer ws.Close()

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

type WsMessage struct {
	Channel string `json:"channel"`
	Body    any    `json:"body"`
}

func (s *WebSocketServer) Send(channel string, in any) {
	message := WsMessage{
		Channel: channel,
	}

	switch v := in.(type) {
	case *model.SensorEventModel:
		message.Body = NewSensorEvent(v)
	case *internal.DeviceState:
		message.Body = NewDeviceEvent(v)
	default:
		slog.Error("not supported type", "input", in)
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
