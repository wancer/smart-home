package mqtt

import (
	"encoding/json"
	"log/slog"
	"smart-home/event"
	"smart-home/internal"
	"sync"
	"time"

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

	//	slog.Debug("sensor", "event", string(msg.Payload()))
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
	c.wg.Add(1)
	defer c.wg.Done()

	slog.Debug("power", "event", string(msg.Payload()))
	device := c.deviceMap.GetByTopic(msg.Topic())

	now := time.Now()
	isOn := string(msg.Payload()) == "ON"
	state := c.states.GetById(device.ID)
	state.On = &isOn
	state.LastUpdate = &now

	c.ws.Send("state", state)
}

func (c *EventHandler) handleState(client mqtt.Client, msg mqtt.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	device := c.deviceMap.GetByTopic(msg.Topic())

	var event *event.Status10
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

	slog.Debug("state", "event", string(msg.Payload()))

	now := time.Now()
	state := c.states.GetById(device.ID)
	state.Current = &event.StatusSNS.Energy.Current
	state.Power = &event.StatusSNS.Energy.Power
	state.Voltage = &event.StatusSNS.Energy.Voltage
	state.LastUpdate = &now

	c.ws.Send("state", state)
}

func (c *EventHandler) handleResult(_ mqtt.Client, msg mqtt.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	slog.Debug("result", "event", string(msg.Payload()), "topic", msg.Topic())
}

func (c *EventHandler) Shutdown() {
	// Wait for the goroutine to finish
	c.wg.Wait()
	slog.Info("Storage flushed")
}
