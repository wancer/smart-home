package mqtt

import (
	"encoding/json"
	"log/slog"
	"maps"
	"slices"
	"smart-home/event"
	"smart-home/internal"
	"sync"
	"time"

	driver "github.com/eclipse/paho.mqtt.golang"
)

type EventHandler struct {
	wg      *sync.WaitGroup
	storage *internal.Storage
	ws      WsBroadcaster // interface
	states  *internal.DeviceStateStorage
}

func NewEventHandler(
	storage *internal.Storage,
	ws WsBroadcaster,
	states *internal.DeviceStateStorage,
) *EventHandler {
	return &EventHandler{
		wg:      &sync.WaitGroup{},
		storage: storage,
		ws:      ws,
		states:  states,
	}
}

func (c *EventHandler) handleSensorEvent(client driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	state := c.states.GetByTopic(msg.Topic())

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

	now := time.Now()
	model := toModel(event, state.Device.ID, &now)

	c.storage.Store(model)
	c.storage.StoreDaily(event, state.Device.ID)

	c.ws.Send("sensor", model)

	state.Current = &event.Energy.Current
	state.Power = &event.Energy.Power
	state.Voltage = &event.Energy.Voltage
	state.Today = &event.Energy.Today
	state.LastUpdate = &now
}

func (c *EventHandler) handlePowerEvent(_ driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	slog.Debug("mqqt-event power", "event", string(msg.Payload()))
	state := c.states.GetByTopic(msg.Topic())

	event, err := event.NewPower(string(msg.Payload()))
	if err != nil {
		slog.Error("POWER_ERR", "error", err)
		return
	}

	now := time.Now()
	state.On = new(bool(event))
	state.LastUpdate = &now

	c.ws.Send("state", state)
}

func (c *EventHandler) handleState(_ driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	state := c.states.GetByTopic(msg.Topic())

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

	slog.Debug("mqqt-event state", "event", string(msg.Payload()))

	now := time.Now()
	state.Current = &event.StatusSNS.Energy.Current
	state.Power = &event.StatusSNS.Energy.Power
	state.Voltage = &event.StatusSNS.Energy.Voltage
	state.Today = &event.StatusSNS.Energy.Today
	state.LastUpdate = &now

	c.ws.Send("state", state)
}

func (c *EventHandler) handleResult(client driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	slog.Debug("mqqt-event result", "event", string(msg.Payload()), "topic", msg.Topic())

	event := map[string]any{}
	err := json.Unmarshal(msg.Payload(), &event)
	if err != nil {
		slog.Error(
			"CANT_PARSE_RESULT",
			"err", err,
			"topic", msg.Topic(),
			"payload", msg.Payload(),
		)
		return
	}

	keys := slices.Collect(maps.Keys(event))
	if len(keys) == 0 {
		slog.Error(
			"EMPTY_RESULT_KEYS",
			"err", err,
			"topic", msg.Topic(),
			"payload", msg.Payload(),
		)
	}

	key := keys[0]
	var val []byte

	switch v := event[key].(type) {
	default:
		val, _ = json.Marshal(event[key])
	case string:
		val = []byte(v)
	}
	pseudoMessage := messageFromResult(msg, val)

	switch key {
	// {"POWER":"ON"}
	// {"LedState":1}
	// {"LedPower1":"OFF"}
	// {"TelePeriod":15}
	// {"Timezone":99}
	// {"LedPwmMode1":"OFF"}
	// {"LedPwmOff":0}
	// {"LedPwmOn":255}
	// {"TimeStd":{"Hemisphere":0,"Week":0,"Month":10,"Day":1,"Hour":4,"Offset":120}}
	// {"TimeDst":{"Hemisphere":0,"Week":0,"Month":3,"Day":1,"Hour":3,"Offset":180}}
	case "POWER":
		c.handlePowerEvent(client, pseudoMessage)
	case "LedPower1":
		c.handleLedPower(client, pseudoMessage)
	case "LedState":
		c.handleLedState(client, pseudoMessage)
	case "TelePeriod":
		c.handleTelePeriod(client, pseudoMessage)
	case "Timezone":
		c.handleTimezone(client, pseudoMessage)
	case "TimeStd":
		c.handleTimeStd(client, pseudoMessage)
	case "TimeDst":
		c.handleTimeDst(client, pseudoMessage)
	case "LedPwmMode1":
		c.handleLedPwmMode(client, pseudoMessage)
	case "LedPwmOff":
		c.handleLedPwmOff(client, pseudoMessage)
	case "LedPwmOn":
		c.handleLedPwmOn(client, pseudoMessage)
	default:
		slog.Warn("mqqt-event result", "event", string(msg.Payload()), "topic", msg.Topic())
	}
}

func (c *EventHandler) handleLedState(_ driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	state := c.states.GetByTopic(msg.Topic())
	parsed, err := event.NewLedState(string(msg.Payload()))
	if err != nil {
		slog.Error("LED_STATE_ERR", "error", err)
		return
	}
	state.Config.LedState = new(uint(parsed))

	slog.Debug("mqqt-event LedState", "event", string(msg.Payload()))
}

func (c *EventHandler) handleLedPower(_ driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	state := c.states.GetByTopic(msg.Topic())
	parsed, err := event.NewLedPwmMode(string(msg.Payload()))
	if err != nil {
		slog.Error("LED_POWER_ERR", "error", err)
		return
	}
	state.Config.LedPower = new(bool(parsed))

	slog.Debug("mqqt-event LedPower1", "event", string(msg.Payload()))
}

func (c *EventHandler) handleTelePeriod(_ driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	state := c.states.GetByTopic(msg.Topic())
	parsed, err := event.NewTelePeriod(string(msg.Payload()))
	if err != nil {
		slog.Error("TELE_PERIOD_ERR", "error", err)
		return
	}
	state.Config.TelePeriod = new(uint(parsed))

	slog.Debug("mqqt-event TelePeriod", "event", string(msg.Payload()))
}

func (c *EventHandler) handleLedPwmMode(_ driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	state := c.states.GetByTopic(msg.Topic())
	parsed, err := event.NewLedPwmMode(string(msg.Payload()))
	if err != nil {
		slog.Error("LED_PWM_MODE_ERR", "error", err)
		return
	}
	state.Config.LedPwmMode = new(bool(parsed))

	slog.Debug("mqqt-event LedPwmMode", "event", string(msg.Payload()))
}

func (c *EventHandler) handleLedPwmOff(_ driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	state := c.states.GetByTopic(msg.Topic())
	parsed, err := event.NewLedPwmOff(string(msg.Payload()))
	if err != nil {
		slog.Error("TELE_PERIOD_ERR", "error", err)
		return
	}
	state.Config.LedPwmOff = new(uint(parsed))

	slog.Debug("mqqt-event LedPwmOff", "event", string(msg.Payload()))
}

func (c *EventHandler) handleLedPwmOn(_ driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	state := c.states.GetByTopic(msg.Topic())
	parsed, err := event.NewLedPwmOn(string(msg.Payload()))
	if err != nil {
		slog.Error("TELE_PERIOD_ERR", "error", err)
		return
	}
	state.Config.LedPwmOn = new(uint(parsed))

	slog.Debug("mqqt-event LedPwmOn", "event", string(msg.Payload()))
}

func (c *EventHandler) handleTimezone(_ driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	state := c.states.GetByTopic(msg.Topic())
	parsed := event.NewTimezone(string(msg.Payload()))
	state.Config.Timezone = new(string(parsed))

	slog.Debug("mqqt-event Timezone", "event", string(msg.Payload()))
}

func (c *EventHandler) handleTimeStd(_ driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	var event event.TimeStd
	err := json.Unmarshal(msg.Payload(), &event)
	if err != nil {
		slog.Error("TIME_STD_ERR", "error", err)
		return
	}
	state := c.states.GetByTopic(msg.Topic())
	state.Config.TimeStd = &internal.TimeSwitch{
		Day:        event.Day,
		Hemisphere: event.Hemisphere,
		Hour:       event.Hour,
		Month:      event.Month,
		Offset:     event.Offset,
		Week:       event.Week,
	}

	slog.Debug("mqqt-event TimeStd", "event", string(msg.Payload()))
}

func (c *EventHandler) handleTimeDst(_ driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	var event event.TimeDst
	err := json.Unmarshal(msg.Payload(), &event)
	if err != nil {
		slog.Error("TIME_DST_ERR", "error", err)
		return
	}
	state := c.states.GetByTopic(msg.Topic())
	state.Config.TimeDst = &internal.TimeSwitch{
		Day:        event.Day,
		Hemisphere: event.Hemisphere,
		Hour:       event.Hour,
		Month:      event.Month,
		Offset:     event.Offset,
		Week:       event.Week,
	}

	slog.Debug("mqqt-event TimeDst", "event", string(msg.Payload()))
}

func (c *EventHandler) handleLogWarning(_ driver.Client, msg driver.Message) {
	c.wg.Add(1)
	defer c.wg.Done()

	slog.Warn("mqqt-event warning", "event", string(msg.Payload()), "topic", msg.Topic())
}

func (c *EventHandler) Shutdown() {
	c.wg.Wait()
	slog.Info("Mqtt handler stopped")
}
