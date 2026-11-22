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
	minRecovery        = time.Minute
	offlineCheckPeriod = 15 * time.Second
	// The "back online" message waits this long at most for the uptime and restart reason
	// polled when the device came online (PublishStates).
	restartInfoWait     = 15 * time.Second
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

// pendingOnline is a "back online" message waiting for the device's uptime and restart reason.
type pendingOnline struct {
	text     string             // default text
	lastSeen time.Time          // last message before it went offline; zero if unknown
	recovery bool               // follow-up of a fired offline alert (always the default text)
	rules    []model.NotifyRule // online rules (may have a custom message)
	timer    *time.Timer
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
	lastSeen     map[uint]time.Time          // device id -> last message before it went offline
	pending      map[uint]*pendingOnline     // device id -> "back online" message waiting for restart info
	alerts       map[uint]*alertState        // rule id -> runtime state
}

func NewNotifier(db *gorm.DB, states *internal.DeviceStateManager, out Messenger) *Notifier {
	return &Notifier{
		db: db, states: states, out: out,
		rules:        map[uint][]model.NotifyRule{},
		offlineSince: map[uint]time.Time{},
		lastSeen:     map[uint]time.Time{},
		pending:      map[uint]*pendingOnline{},
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
	dispatcher.Subscribe("DeviceRestartInfo", func(raw any) {
		n.onRestartInfo(raw.(*event.DeviceRestartInfo))
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

	// A message still waiting for restart info goes out now, before anything else about the device.
	n.flushOnline(e.DeviceId)

	// Disabling a device makes the monitor mark it offline: that is not worth a message.
	if state == nil || !state.Device.Enabled {
		delete(n.offlineSince, e.DeviceId)
		delete(n.lastSeen, e.DeviceId)
		for _, rule := range n.rules[e.DeviceId] {
			delete(n.alerts, rule.ID)
		}
		return
	}

	if !e.Online {
		n.offlineSince[e.DeviceId] = now
		delete(n.lastSeen, e.DeviceId)
		if state.LastUpdate != nil {
			n.lastSeen[e.DeviceId] = *state.LastUpdate
		}
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
	lastSeen := n.lastSeen[e.DeviceId]
	delete(n.offlineSince, e.DeviceId)
	delete(n.lastSeen, e.DeviceId)
	down := now.Sub(since)
	p := &pendingOnline{
		text: fmt.Sprintf("%s %s is back online (was offline for %s)",
			notifierEmojiOnline, state.Device.Name, humanDuration(down)),
		lastSeen: lastSeen,
	}
	for _, rule := range n.rules[e.DeviceId] {
		switch rule.Kind {
		case model.NotifyOffline:
			// Only follow up an offline alert that was actually sent.
			if a := n.alerts[rule.ID]; a != nil && a.firing && rule.NotifyRecovery && wasOffline {
				p.recovery = true
			}
			delete(n.alerts, rule.ID)
		case model.NotifyOnline:
			if wasOffline && down >= rule.Duration() {
				p.rules = append(p.rules, rule)
			}
		}
	}
	if !p.recovery && len(p.rules) == 0 {
		return
	}

	// Wait for the uptime and restart reason (polled on the online transition),
	// so the message tells a restart from a lost connection.
	deviceId := e.DeviceId
	p.timer = time.AfterFunc(restartInfoWait, func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		if n.pending[deviceId] == p {
			n.flushOnline(deviceId)
		}
	})
	n.pending[deviceId] = p
	if hasRestartInfo(state) {
		n.flushOnline(deviceId)
	}
}

func (n *Notifier) onRestartInfo(e *event.DeviceRestartInfo) {
	state := n.states.GetById(e.DeviceId)
	if state == nil || !hasRestartInfo(state) {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.flushOnline(e.DeviceId)
}

func hasRestartInfo(state *internal.DeviceState) bool {
	return state.Uptime != nil && state.RestartReason != nil
}

// flushOnline sends the device's pending "back online" message, with whatever restart
// info is known by now. Must be called with n.mu held.
func (n *Notifier) flushOnline(deviceId uint) {
	p := n.pending[deviceId]
	if p == nil {
		return
	}
	delete(n.pending, deviceId)
	p.timer.Stop()

	details := ""
	if state := n.states.GetById(deviceId); state != nil {
		details = restartDetails(state, p.lastSeen, time.Now())
	}
	if p.recovery {
		n.out.Notify(p.text + details)
	}
	for _, rule := range p.rules {
		n.sendWithDetails(rule, p.text, details)
	}
}

// restartDetails tells whether the device restarted while it was away (its uptime is
// shorter than the time since it was last seen) or just lost the connection.
func restartDetails(state *internal.DeviceState, lastSeen, now time.Time) string {
	var lines []string
	reasonLabel := "Restart reason"
	if state.Uptime != nil {
		up := time.Duration(state.Uptime.Current()) * time.Second
		line := "Uptime: " + humanDuration(up)
		if !lastSeen.IsZero() {
			if up < now.Sub(lastSeen) {
				line += " — device restarted"
			} else {
				line += " — no restart, connection was lost"
				reasonLabel = "Last restart reason"
			}
		}
		lines = append(lines, line)
	}
	if state.RestartReason != nil && *state.RestartReason != "" {
		lines = append(lines, reasonLabel+": "+*state.RestartReason)
	}
	if len(lines) == 0 {
		return ""
	}
	return "\n" + strings.Join(lines, "\n")
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
	// A switched-off relay reads 0 W / 0 A: that is normal, not "below threshold".
	switchedOff := state.CanToggle() && state.On != nil && !*state.On

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
		offNormal := switchedOff && relayMetric(rule.Metric)
		holds := !offNormal &&
			((rule.Kind == model.NotifyAbove && value > rule.Threshold) ||
				(rule.Kind == model.NotifyBelow && value < rule.Threshold))

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
				if offNormal {
					n.out.Notify(fmt.Sprintf("%s %s: switched off", notifierEmojiOk, state.Device.Name))
				} else {
					n.out.Notify(fmt.Sprintf("%s %s: %s is back to normal, %s",
						notifierEmojiOk, state.Device.Name, rule.Metric, formatMetric(rule.Metric, value)))
				}
			}
		}
	}
}

// send delivers the rule's custom message if it has one, the default text otherwise.
func (n *Notifier) send(rule model.NotifyRule, defaultText string) {
	n.sendWithDetails(rule, defaultText, "")
}

// sendWithDetails is send with details appended to either text.
func (n *Notifier) sendWithDetails(rule model.NotifyRule, defaultText, details string) {
	if rule.Message != "" {
		state := n.states.GetById(rule.DeviceId)
		if state != nil {
			n.out.Notify(state.Device.Name + ": " + rule.Message + details)
			return
		}
	}
	n.out.Notify(defaultText + details)
}

// relayMetric reports whether the metric drops to 0 when the relay is switched off.
func relayMetric(metric string) bool {
	return metric == model.MetricPower || metric == model.MetricCurrent
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
	case model.MetricPressure:
		if state.Pressure != nil {
			return float64(*state.Pressure), true
		}
	case model.MetricGas:
		if state.Gas != nil {
			return float64(*state.Gas), true
		}
	case model.MetricDcVoltage:
		if state.DcVoltage != nil {
			return float64(*state.DcVoltage), true
		}
	case model.MetricDcCurrent:
		if state.DcCurrent != nil {
			return float64(*state.DcCurrent), true
		}
	case model.MetricDcPower:
		if state.DcPower != nil {
			return float64(*state.DcPower), true
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
	model.MetricPressure:    {" hPa", 1},
	model.MetricGas:         {" ppm", 2},
	model.MetricDcVoltage:   {" V", 2},
	model.MetricDcCurrent:   {" A", 3},
	model.MetricDcPower:     {" W", 2},
}

func formatMetric(metric string, v float64) string {
	u := metricUnits[metric]
	return strconv.FormatFloat(v, 'f', u.precision, 64) + u.unit
}

// humanDuration renders e.g. "3d 4h", "1h 5m" or "45s".
func humanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	days, h, m, s := int(d.Hours())/24, int(d.Hours())%24, int(d.Minutes())%60, int(d.Seconds())%60
	var parts []string
	if days > 0 {
		parts = append(parts, strconv.Itoa(days)+"d")
	}
	if h > 0 {
		parts = append(parts, strconv.Itoa(h)+"h")
	}
	if m > 0 && days == 0 {
		parts = append(parts, strconv.Itoa(m)+"m")
	}
	if s > 0 && h == 0 && days == 0 {
		parts = append(parts, strconv.Itoa(s)+"s")
	}
	if len(parts) == 0 {
		return "0s"
	}
	return strings.Join(parts, " ")
}
