package mqtt

import (
	"encoding/json"
	"log/slog"
	"smart-home/event"
	"smart-home/internal"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type EventHandler struct {
	wg        *sync.WaitGroup
	deviceMap *internal.DeviceMap
	storage   *internal.Storage
	ws        WsBroadcaster // interface
}

func NewEventHandler(
	deviceMap *internal.DeviceMap,
	storage *internal.Storage,
	ws WsBroadcaster,
) *EventHandler {
	return &EventHandler{
		wg:        &sync.WaitGroup{},
		deviceMap: deviceMap,
		storage:   storage,
		ws:        ws,
	}
}

func (c *EventHandler) handleSensorEvent(client mqtt.Client, msg mqtt.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	device := c.deviceMap.GetByTopic(msg.Topic())

	var event *event.SensorEvent
	err := json.Unmarshal(msg.Payload(), &event)
	if err != nil {
		slog.Error(
			"CANT_PARSE_MSG",
			"err", err,
			"topic", msg.Topic(),
			"payload", msg.Payload(),
		)
		return
	}

	slog.Debug("sensor", "event", string(msg.Payload()))
	m := toModel(event, device)

	c.storage.Store(m)
	c.storage.StoreDaily(event, device)

	c.ws.Send("sensor", m)
}

func (c *EventHandler) handlePowerEvent(client mqtt.Client, msg mqtt.Message) {
	slog.Info("sensor", "event", string(msg.Payload()), "topic", msg.Topic())
}

func (c *EventHandler) Shutdown() {
	// Wait for the goroutine to finish
	c.wg.Wait()
	slog.Info("Storage flushed")
}
