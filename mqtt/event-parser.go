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

func (c *EventParser) parsePowerEvent(_ driver.Client, msg driver.Message) {
	parsed, err := event.NewPower(string(msg.Payload()))
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

	key := keys[0]

	var parsed any

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
		var e event.Power
		e, err = event.NewPower(rawMapped[key].(string))
		parsed = &e
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
