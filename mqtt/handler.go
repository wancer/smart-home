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
	states    *internal.StateStorage
}

func NewEventHandler(
	deviceMap *internal.DeviceMap,
	storage *internal.Storage,
	ws WsBroadcaster,
	states *internal.StateStorage,
) *EventHandler {
	return &EventHandler{
		wg:        &sync.WaitGroup{},
		deviceMap: deviceMap,
		storage:   storage,
		ws:        ws,
		states:    states,
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
	model := toModel(event, device)

	c.storage.Store(model)
	c.storage.StoreDaily(event, device)

	c.ws.Send("sensor", model)

	state := c.states.GetById(device.ID)
	state.Current = &model.Current
	state.Power = &model.Power
	state.Voltage = &model.Voltage
	state.LastUpdate = &model.RealTime
}

func (c *EventHandler) handlePowerEvent(client mqtt.Client, msg mqtt.Message) {
	device := c.deviceMap.GetByTopic(msg.Topic())

	isOn := string(msg.Payload()) == "ON"
	state := c.states.GetById(device.ID)
	state.On = &isOn

	c.ws.Send("state", state)
}

func (c *EventHandler) Shutdown() {
	// Wait for the goroutine to finish
	c.wg.Wait()
	slog.Info("Storage flushed")
}
