package automation

import (
	"fmt"
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

const (
	MaxNotifyDuration = 7 * 24 * time.Hour
	// A fired threshold alert re-arms only after the value stayed normal this long
	// (or the rule's own duration, if longer), so a value hovering around the
	// threshold does not flood the chat.
	minRecovery         = time.Minute
	offlineCheckPeriod  = 15 * time.Second
	notifierTimeFormat  = "15:04"
	notifierEmojiAlert  = "⚠️"
	notifierEmojiOk     = "✅"
	notifierEmojiOnline = "🟢"
	notifierEmojiOff    = "🔴"
)

// Messenger delivers notification text (Telegram).
type Messenger interface {
	Notify(text string)
}

type alertState struct {
	pendingSince time.Time // condition holds since; zero when not pending
	clearSince   time.Time // firing alert's condition is false since; zero when not clearing
	firing       bool
}

// Notifier evaluates per-device NotifyRules against availability and sensor events.
type Notifier struct {
	db     *gorm.DB
	states *internal.DeviceStateManager
	out    Messenger

	mu           sync.Mutex
	rules        map[uint][]model.NotifyRule // device id -> enabled rules
	offlineSince map[uint]time.Time          // device id -> when it went offline
	alerts       map[uint]*alertState        // rule id -> runtime state
}

func NewNotifier(db *gorm.DB, states *internal.DeviceStateManager, out Messenger) *Notifier {
	return &Notifier{
		db: db, states: states, out: out,
		rules:        map[uint][]model.NotifyRule{},
		offlineSince: map[uint]time.Time{},
		alerts:       map[uint]*alertState{},
	}
}

// Reload re-reads enabled rules from the database. Runtime state is kept for
// rules that are unchanged, and dropped for rules that were edited or removed.
func (n *Notifier) Reload() error {
	var rows []model.NotifyRule
	if err := n.db.Where("enabled = ?", true).Find(&rows).Error; err != nil {
		return err
	}

	byDevice := map[uint][]model.NotifyRule{}
	byId := map[uint]model.NotifyRule{}
	for _, r := range rows {
		byDevice[r.DeviceId] = append(byDevice[r.DeviceId], r)
		byId[r.ID] = r
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	for _, list := range n.rules {
		for _, old := range list {
			if cur, ok := byId[old.ID]; !ok || cur != old {
				delete(n.alerts, old.ID)
			}
		}
	}
	n.rules = byDevice
	return nil
}

// Active reports whether a threshold/offline rule has fired and not yet recovered.
func (n *Notifier) Active(ruleId uint) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	a := n.alerts[ruleId]
	return a != nil && a.firing
}

func (n *Notifier) Subscribe(dispatcher *event.Dispatcher) {
	dispatcher.Subscribe("DeviceAvailability", func(raw any) {
		n.onAvailability(raw.(*event.DeviceAvailability))
	})
	dispatcher.Subscribe("SensorsUpdated", func(raw any) {
		n.onSensors(raw.(*event.SensorsUpdated))
	})
}

// Run fires offline rules that have a duration once the device stayed offline long enough.
func (n *Notifier) Run() chan struct{} {
	ticker := time.NewTicker(offlineCheckPeriod)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				n.checkOffline(time.Now())
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
	return quit
}

func (n *Notifier) alert(ruleId uint) *alertState {
	a, ok := n.alerts[ruleId]
	if !ok {
		a = &alertState{}
		n.alerts[ruleId] = a
	}
	return a
}

func (n *Notifier) onAvailability(e *event.DeviceAvailability) {
	state := n.states.GetById(e.DeviceId)
	now := time.Now()

	n.mu.Lock()
	defer n.mu.Unlock()

	// Disabling a device makes the monitor mark it offline: that is not worth a message.
	if state == nil || !state.Device.Enabled {
		delete(n.offlineSince, e.DeviceId)
		return
	}

	if !e.Online {
		n.offlineSince[e.DeviceId] = now
		for _, rule := range n.rules[e.DeviceId] {
			a := n.alert(rule.ID)
			// readings stop while offline: duration windows restart once it is back
			a.pendingSince, a.clearSince = time.Time{}, time.Time{}
		}
		n.checkDeviceOffline(state, now, now)
		return
	}

	// Without a recorded offline period (e.g. first telemetry after a restart) there is nothing to report.
	since, wasOffline := n.offlineSince[e.DeviceId]
	delete(n.offlineSince, e.DeviceId)
	for _, rule := range n.rules[e.DeviceId] {
		switch rule.Kind {
		case model.NotifyOffline:
			delete(n.alerts, rule.ID)
		case model.NotifyOnline:
			down := now.Sub(since)
			if wasOffline && down >= rule.Duration() {
				n.send(rule, fmt.Sprintf("%s %s is back online (was offline for %s)",
					notifierEmojiOnline, state.Device.Name, humanDuration(down)))
			}
		}
	}
}

func (n *Notifier) checkOffline(now time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for deviceId, since := range n.offlineSince {
		if state := n.states.GetById(deviceId); state != nil && state.Device.Enabled {
			n.checkDeviceOffline(state, since, now)
		}
	}
}

// checkDeviceOffline must be called with n.mu held.
func (n *Notifier) checkDeviceOffline(state *internal.DeviceState, since, now time.Time) {
	for _, rule := range n.rules[state.Device.ID] {
		if rule.Kind != model.NotifyOffline {
			continue
		}
		a := n.alert(rule.ID)
		if a.firing || now.Sub(since) < rule.Duration() {
			continue
		}
		a.firing = true

		text := fmt.Sprintf("%s %s is offline", notifierEmojiOff, state.Device.Name)
		if rule.DurationSec > 0 {
			text += " for " + humanDuration(now.Sub(since))
		}
		if state.LastUpdate != nil {
			text += " (last seen " + state.LastUpdate.Format(notifierTimeFormat) + ")"
		}
		n.send(rule, text)
	}
}

func (n *Notifier) onSensors(e *event.SensorsUpdated) {
	state := n.states.GetById(e.DeviceId)
	if state == nil || !state.Device.Enabled {
		return
	}
	now := time.Now()

	n.mu.Lock()
	defer n.mu.Unlock()

	for _, rule := range n.rules[e.DeviceId] {
		if rule.Kind != model.NotifyAbove && rule.Kind != model.NotifyBelow {
			continue
		}
		value, ok := metricValue(state, rule.Metric)
		if !ok {
			continue
		}

		a := n.alert(rule.ID)
		holds := (rule.Kind == model.NotifyAbove && value > rule.Threshold) ||
			(rule.Kind == model.NotifyBelow && value < rule.Threshold)

		if holds {
			a.clearSince = time.Time{}
			if a.firing {
				continue
			}
			if a.pendingSince.IsZero() {
				a.pendingSince = now
			}
			if held := now.Sub(a.pendingSince); held >= rule.Duration() {
				a.firing, a.pendingSince = true, time.Time{}
				text := fmt.Sprintf("%s %s: %s %s is %s %s",
					notifierEmojiAlert, state.Device.Name, rule.Metric,
					formatMetric(rule.Metric, value), rule.Kind, formatMetric(rule.Metric, rule.Threshold))
				if rule.DurationSec > 0 {
					text += " for " + humanDuration(held)
				}
				n.send(rule, text)
			}
			continue
		}

		a.pendingSince = time.Time{}
		if !a.firing {
			continue
		}
		if a.clearSince.IsZero() {
			a.clearSince = now
		}
		if now.Sub(a.clearSince) >= max(rule.Duration(), minRecovery) {
			a.firing, a.clearSince = false, time.Time{}
			if rule.NotifyRecovery {
				n.out.Notify(fmt.Sprintf("%s %s: %s is back to normal, %s",
					notifierEmojiOk, state.Device.Name, rule.Metric, formatMetric(rule.Metric, value)))
			}
		}
	}
}

// send delivers the rule's custom message if it has one, the default text otherwise.
func (n *Notifier) send(rule model.NotifyRule, defaultText string) {
	if rule.Message != "" {
		state := n.states.GetById(rule.DeviceId)
		if state != nil {
			n.out.Notify(state.Device.Name + ": " + rule.Message)
			return
		}
	}
	n.out.Notify(defaultText)
}

func metricValue(state *internal.DeviceState, metric string) (float64, bool) {
	switch metric {
	case model.MetricPower:
		if state.Power != nil {
			return float64(*state.Power), true
		}
	case model.MetricCurrent:
		if state.Current != nil {
			return float64(*state.Current), true
		}
	case model.MetricVoltage:
		if state.Voltage != nil {
			return float64(*state.Voltage), true
		}
	case model.MetricCO2:
		if state.CO2 != nil {
			return float64(*state.CO2), true
		}
	case model.MetricTemperature:
		if state.Temperature != nil {
			return float64(*state.Temperature), true
		}
	case model.MetricHumidity:
		if state.Humidity != nil {
			return float64(*state.Humidity), true
		}
	}
	return 0, false
}

var metricUnits = map[string]struct {
	unit      string
	precision int
}{
	model.MetricPower:       {" W", 0},
	model.MetricCurrent:     {" A", 2},
	model.MetricVoltage:     {" V", 0},
	model.MetricCO2:         {" ppm", 0},
	model.MetricTemperature: {"°C", 1},
	model.MetricHumidity:    {"%", 1},
}

func formatMetric(metric string, v float64) string {
	u := metricUnits[metric]
	return strconv.FormatFloat(v, 'f', u.precision, 64) + u.unit
}

// humanDuration renders e.g. "1h 5m" or "45s".
func humanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	var parts []string
	if h > 0 {
		parts = append(parts, strconv.Itoa(h)+"h")
	}
	if m > 0 {
		parts = append(parts, strconv.Itoa(m)+"m")
	}
	if s > 0 && h == 0 {
		parts = append(parts, strconv.Itoa(s)+"s")
	}
	if len(parts) == 0 {
		return "0s"
	}
	return strings.Join(parts, " ")
}
