package mqtt

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"smart-home/event"
	"strconv"
	"strings"

	driver "github.com/eclipse/paho.mqtt.golang"
)

type EventParser struct {
	dispatcher *event.Dispatcher
}

func NewEventParser(d *event.Dispatcher) *EventParser {
	return &EventParser{dispatcher: d}
}

func (c *EventParser) parseSensorEvent(_ driver.Client, msg driver.Message) {
	var parsed event.SensorEvent
	err := json.Unmarshal(msg.Payload(), &parsed)
	if err != nil {
		slog.Error(
			"CANT_PARSE_MSG",
			"err", err,
			"topic", msg.Topic(),
			"payload", msg.Payload(),
		)
		return
	}

	slog.Debug("[event] sensor", "event", string(msg.Payload()))

	c.dispatcher.DispatchMqqt(&parsed, msg.Topic())
}

func (c *EventParser) parseLwt(_ driver.Client, msg driver.Message) {
	parsed, err := event.NewLwt(string(msg.Payload()))
	if err != nil {
		slog.Error("LWT_ERR", "error", err, "topic", msg.Topic())
		return
	}

	slog.Debug("[event] lwt", "event", string(msg.Payload()))
	c.dispatcher.DispatchMqqt(&parsed, msg.Topic())
}

func (c *EventParser) parseState(_ driver.Client, msg driver.Message) {
	var parsed event.Status10
	err := json.Unmarshal(msg.Payload(), &parsed)
	if err != nil {
		slog.Error(
			"CANT_PARSE_MSG",
			"err", err,
			"topic", msg.Topic(),
			"payload", msg.Payload(),
		)
		return
	}

	slog.Debug("[event] state", "event", string(msg.Payload()))

	c.dispatcher.DispatchMqqt(&parsed, msg.Topic())
}

// parseInfo handles the boot messages tele/<topic>/INFO1 and INFO3 (ESPHome devices only).
func (c *EventParser) parseInfo(_ driver.Client, msg driver.Message) {
	var parsed event.Info
	if err := json.Unmarshal(msg.Payload(), &parsed); err != nil {
		slog.Error("CANT_PARSE_MSG", "err", err, "topic", msg.Topic(), "payload", string(msg.Payload()))
		return
	}

	slog.Debug("[event] info", "event", string(msg.Payload()))
	c.dispatcher.DispatchMqqt(&parsed, msg.Topic())
}

// parseTeleState handles the periodic tele/<topic>/STATE message.
func (c *EventParser) parseTeleState(_ driver.Client, msg driver.Message) {
	var parsed event.TeleState
	if err := json.Unmarshal(msg.Payload(), &parsed); err != nil {
		slog.Error("CANT_PARSE_MSG", "err", err, "topic", msg.Topic(), "payload", string(msg.Payload()))
		return
	}

	slog.Debug("[event] tele state", "event", string(msg.Payload()))
	c.dispatcher.DispatchMqqt(&parsed, msg.Topic())
	c.dispatchTelePower(msg.Payload(), "", msg.Topic())
}

// dispatchTelePower sends the POWER<n> keys of a tele/STATE payload (under key, if set) as a Power event.
func (c *EventParser) dispatchTelePower(payload []byte, key string, topic string) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return
	}
	if key != "" {
		raw, _ = raw[key].(map[string]any)
	}
	if p, ok := event.NewPowerFromMap(raw); ok {
		p.Tele = true
		c.dispatcher.DispatchMqqt(p, topic)
	}
}

// parseStatus11 handles the STATUS11 reply: the tele/STATE payload wrapped in StatusSTS.
func (c *EventParser) parseStatus11(_ driver.Client, msg driver.Message) {
	var parsed event.Status11
	if err := json.Unmarshal(msg.Payload(), &parsed); err != nil || parsed.StatusSTS == nil {
		slog.Error("CANT_PARSE_MSG", "err", err, "topic", msg.Topic(), "payload", string(msg.Payload()))
		return
	}

	slog.Debug("[event] status11", "event", string(msg.Payload()))
	c.dispatcher.DispatchMqqt(parsed.StatusSTS, msg.Topic())
	c.dispatchTelePower(msg.Payload(), "StatusSTS", msg.Topic())
}

// parseStatus1 handles the STATUS1 reply (restart reason).
func (c *EventParser) parseStatus1(_ driver.Client, msg driver.Message) {
	var parsed event.Status1
	if err := json.Unmarshal(msg.Payload(), &parsed); err != nil || parsed.StatusPRM == nil {
		slog.Error("CANT_PARSE_MSG", "err", err, "topic", msg.Topic(), "payload", string(msg.Payload()))
		return
	}

	slog.Debug("[event] status1", "event", string(msg.Payload()))
	c.dispatcher.DispatchMqqt(&parsed, msg.Topic())
}

func (c *EventParser) parseFirmware(_ driver.Client, msg driver.Message) {
	var parsed event.Status2
	err := json.Unmarshal(msg.Payload(), &parsed)
	if err != nil {
		slog.Error(
			"CANT_PARSE_MSG",
			"err", err,
			"topic", msg.Topic(),
			"payload", msg.Payload(),
		)
		return
	}

	slog.Debug("[event] firmware", "event", string(msg.Payload()))
	c.dispatcher.DispatchMqqt(&parsed, msg.Topic())
}

// parsePowerEvent handles stat/<topic>/POWER (and POWER<n>, if subscribed).
func (c *EventParser) parsePowerEvent(_ driver.Client, msg driver.Message) {
	topic := msg.Topic()
	index, ok := event.PowerKeyIndex(topic[strings.LastIndex(topic, "/")+1:])
	if !ok {
		index = 1
	}
	parsed, err := event.NewPower(string(msg.Payload()), index)
	if err != nil {
		slog.Error("POWER_ERR", "error", err)
		return
	}

	slog.Debug("[event] power", "event", string(msg.Payload()))

	c.dispatcher.DispatchMqqt(&parsed, msg.Topic())
}

func (c *EventParser) parseResult(_ driver.Client, msg driver.Message) {
	slog.Debug("[event] result", "event", string(msg.Payload()), "topic", msg.Topic())

	rawMapped := map[string]any{}
	err := json.Unmarshal(msg.Payload(), &rawMapped)
	if err != nil {
		slog.Error("CANT_PARSE_RESULT", "err", err, "topic", msg.Topic(), "payload", msg.Payload())
		return
	}

	keys := slices.Collect(maps.Keys(rawMapped))
	if len(keys) == 0 {
		slog.Error("EMPTY_RESULT_KEYS", "err", err, "topic", msg.Topic(), "payload", msg.Payload())
		return
	}

	// Multi-key replies: check these before picking a single key
	if _, ok := rawMapped["GPIO"].([]any); ok {
		e := event.NewTemplate(rawMapped)
		c.dispatcher.DispatchMqqt(&e, msg.Topic())
		return
	}
	// {"POWER":"ON"}, {"POWER2":"OFF"}, {"POWER1":"ON","POWER2":"OFF",…}: relays (or the light's power)
	if event.OnlyPowerKeys(rawMapped) {
		if p, ok := event.NewPowerFromMap(rawMapped); ok {
			c.dispatcher.DispatchMqqt(p, msg.Topic())
		} else {
			slog.Warn("mqqt-event result", "event", string(msg.Payload()), "topic", msg.Topic())
		}
		return
	}
	if light, ok := event.NewLight(rawMapped); ok {
		c.dispatcher.DispatchMqqt(light, msg.Topic())
		return
	}

	key := keys[0]

	var parsed any

	switch key {
	// {"LedState":1}
	// {"LedPower1":"OFF"}
	// {"TelePeriod":15}
	// {"Timezone":99}
	// {"LedPwmMode1":"OFF"}
	// {"LedPwmOff":0}
	// {"LedPwmOn":255}
	// {"TimeStd":{"Hemisphere":0,"Week":0,"Month":10,"Day":1,"Hour":4,"Offset":120}}
	// {"TimeDst":{"Hemisphere":0,"Week":0,"Month":3,"Day":1,"Hour":3,"Offset":180}}
	case "LedPower1":
		var e event.LedPower
		e, err = event.NewLedPower(rawMapped[key].(string))
		parsed = &e
	case "LedState":
		var e event.LedState
		formatted := fmt.Sprintf("%.0f", rawMapped[key])
		e, err = event.NewLedState(formatted)
		parsed = &e
	case "LedPwmMode1":
		var e event.LedPwmMode
		e, err = event.NewLedPwmMode(rawMapped[key].(string))
		parsed = &e
	case "LedPwmOff":
		var e event.LedPwmOff
		formatted := fmt.Sprintf("%.0f", rawMapped[key])
		e, err = event.NewLedPwmOff(formatted)
		parsed = &e
	case "LedPwmOn":
		var e event.LedPwmOn
		formatted := fmt.Sprintf("%.0f", rawMapped[key])
		e, err = event.NewLedPwmOn(formatted)
		parsed = &e
	case "TelePeriod":
		var e event.TelePeriod
		formatted := fmt.Sprintf("%.0f", rawMapped[key])
		e, err = event.NewTelePeriod(formatted)
		parsed = &e
	case "Timezone":
		formatted := fmt.Sprintf("%.0f", rawMapped[key])
		e := event.NewTimezone(formatted)
		parsed = &e
	case "TimeStd":
		v := rawMapped[key]
		e := v.(map[string]any)
		p := event.NewTimeStd(e)
		parsed = &p
	case "TimeDst":
		v := rawMapped[key]
		e := v.(map[string]any)
		p := event.NewTimeDst(e)
		parsed = &p
	default:
		if strings.HasPrefix(key, "Timer") {
			n, numErr := strconv.Atoi(key[5:])
			if numErr != nil || n < 1 || n > 16 {
				slog.Warn("mqqt-event result", "event", string(msg.Payload()), "topic", msg.Topic())
				return
			}
			timerData, ok := rawMapped[key].(map[string]any)
			if !ok {
				slog.Warn("mqqt-event result", "event", string(msg.Payload()), "topic", msg.Topic())
				return
			}
			p := event.NewTimer(n, timerData)
			parsed = &p
		} else if strings.HasPrefix(key, "Rule") && len(key) == 5 {
			n, numErr := strconv.Atoi(key[4:])
			if numErr != nil || n < 1 || n > 3 {
				slog.Warn("mqqt-event result", "event", string(msg.Payload()), "topic", msg.Topic())
				return
			}
			ruleData, ok := rawMapped[key].(map[string]any)
			if !ok {
				slog.Warn("mqqt-event result", "event", string(msg.Payload()), "topic", msg.Topic())
				return
			}
			p := event.NewRule(n, ruleData)
			parsed = &p
		} else {
			slog.Warn("mqqt-event result", "event", string(msg.Payload()), "topic", msg.Topic())
			return
		}
	}

	if err != nil {
		slog.Error("CANT_PARSE_RESULT", "err", err, "topic", msg.Topic(), "payload", msg.Payload())
		return
	}

	c.dispatcher.DispatchMqqt(parsed, msg.Topic())
}

func (c *EventParser) parseTimers(_ driver.Client, msg driver.Message) {
	slog.Debug("[event] timers", "event", string(msg.Payload()))

	var raw map[string]any
	if err := json.Unmarshal(msg.Payload(), &raw); err != nil {
		slog.Error("CANT_PARSE_TIMERS", "err", err, "payload", string(msg.Payload()))
		return
	}

	// Format: {"Timers":{"Timer1":{...},...}} or flat {"Timer1":{...},...}
	source := raw
	if nested, ok := raw["Timers"].(map[string]any); ok {
		source = nested
	}

	for k, v := range source {
		if !strings.HasPrefix(k, "Timer") {
			continue
		}
		n, err := strconv.Atoi(k[5:])
		if err != nil || n < 1 || n > 16 {
			continue
		}
		timerData, ok := v.(map[string]any)
		if !ok {
			continue
		}
		p := event.NewTimer(n, timerData)
		c.dispatcher.DispatchMqqt(&p, msg.Topic())
	}
}

func (c *EventParser) parseAsWarning(_ driver.Client, msg driver.Message) {
	slog.Warn("mqqt-event warning", "event", string(msg.Payload()), "topic", msg.Topic())
}

// parseTeleResult handles unsolicited tele/<topic>/RESULT messages (currently only IrReceived).
func (c *EventParser) parseTeleResult(_ driver.Client, msg driver.Message) {
	var raw struct {
		IrReceived *event.IrReceived `json:"IrReceived"`
	}
	if err := json.Unmarshal(msg.Payload(), &raw); err != nil {
		slog.Error("CANT_PARSE_TELE_RESULT", "err", err, "topic", msg.Topic(), "payload", string(msg.Payload()))
		return
	}
	if raw.IrReceived == nil {
		slog.Debug("[event] tele result ignored", "topic", msg.Topic(), "payload", string(msg.Payload()))
		return
	}

	slog.Debug("[event] ir received", "event", string(msg.Payload()))
	c.dispatcher.DispatchMqqt(raw.IrReceived, msg.Topic())
}

// parseMatterInfo handles tele/<sensor>/INFO of a Matter bridge (vendor, product, versions).
func (c *EventParser) parseMatterInfo(_ driver.Client, msg driver.Message) {
	var parsed event.MatterInfo
	if err := json.Unmarshal(msg.Payload(), &parsed); err != nil {
		slog.Error("CANT_PARSE_MSG", "err", err, "topic", msg.Topic(), "payload", string(msg.Payload()))
		return
	}

	slog.Debug("[event] matter info", "event", string(msg.Payload()))
	c.dispatcher.DispatchMqqt(&parsed, msg.Topic())
}

// parseCommandResult handles stat/<sensor>/RESULT of a Matter bridge: the reply to a command.
func (c *EventParser) parseCommandResult(_ driver.Client, msg driver.Message) {
	var raw map[string]any
	if err := json.Unmarshal(msg.Payload(), &raw); err != nil {
		slog.Error("CANT_PARSE_RESULT", "err", err, "topic", msg.Topic(), "payload", string(msg.Payload()))
		return
	}
	parsed, ok := event.NewCommandResult(raw)
	if !ok {
		slog.Warn("mqqt-event result", "event", string(msg.Payload()), "topic", msg.Topic())
		return
	}

	slog.Debug("[event] command result", "event", string(msg.Payload()))
	c.dispatcher.DispatchMqqt(parsed, msg.Topic())
}
