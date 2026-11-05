package automation

import (
	"log/slog"
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Publisher is the subset of mqtt.Publisher the runner needs.
type Publisher interface {
	OnOff(device *model.Device, state bool)
	SendIR(device *model.Device, protocol string, bits int, code string)
}

type ReceivedCode struct {
	Code     string `json:"code"`
	Protocol string `json:"protocol"`
	Bits     int    `json:"bits"`
	Time     int64  `json:"time"`
}

const (
	debounce     = time.Second
	maxReceived  = 10
	MaxDelayMs   = 60000
	MaxRuleSteps = 20
)

// IrRuleRunner reacts to incoming IR codes and executes matching rules.
type IrRuleRunner struct {
	db     *gorm.DB
	states *internal.DeviceStateManager
	pub    Publisher

	mu       sync.Mutex
	lastRun  map[uint]time.Time // rule id -> last start
	running  map[uint]bool
	received map[uint][]ReceivedCode // device id -> latest codes (newest first)
}

func NewIrRuleRunner(db *gorm.DB, states *internal.DeviceStateManager, pub Publisher) *IrRuleRunner {
	return &IrRuleRunner{
		db: db, states: states, pub: pub,
		lastRun:  map[uint]time.Time{},
		running:  map[uint]bool{},
		received: map[uint][]ReceivedCode{},
	}
}

// Received returns the most recent codes seen by the device (for "learn" in the UI).
func (r *IrRuleRunner) Received(deviceId uint) []ReceivedCode {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ReceivedCode{}, r.received[deviceId]...)
}

func (r *IrRuleRunner) Handle(state *internal.DeviceState, e *event.IrReceived) {
	if state == nil || state.Device.SensorType != model.SensorTypeIR {
		return
	}
	// Repeat frames are sent while a button is held; act on the first press only.
	if e.Repeat > 0 {
		return
	}

	code := e.NormalizedCode()
	deviceId := state.Device.ID
	slog.Info("[ir][received]", "device", deviceId, "code", code, "protocol", e.Protocol)

	r.mu.Lock()
	list := append([]ReceivedCode{{Code: code, Protocol: e.Protocol, Bits: e.Bits, Time: time.Now().Unix()}}, r.received[deviceId]...)
	if len(list) > maxReceived {
		list = list[:maxReceived]
	}
	r.received[deviceId] = list
	r.mu.Unlock()

	var rules []model.IrRule
	if err := r.db.Where("device_id = ? AND code = ? AND enabled = ?", deviceId, code, true).Find(&rules).Error; err != nil {
		slog.Error("[ir][rules] load error", "err", err)
		return
	}

	for _, rule := range rules {
		if !r.begin(rule.ID) {
			continue
		}
		go func() {
			defer r.finish(rule.ID)
			r.execute(rule)
		}()
	}
}

// begin guards against re-triggering a rule that is still running or was just fired.
func (r *IrRuleRunner) begin(ruleId uint) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running[ruleId] || time.Since(r.lastRun[ruleId]) < debounce {
		return false
	}
	r.running[ruleId] = true
	r.lastRun[ruleId] = time.Now()
	return true
}

func (r *IrRuleRunner) finish(ruleId uint) {
	r.mu.Lock()
	delete(r.running, ruleId)
	r.mu.Unlock()
}

func (r *IrRuleRunner) execute(rule model.IrRule) {
	var actions []model.IrRuleAction
	if err := r.db.Where("rule_id = ?", rule.ID).Order("position").Find(&actions).Error; err != nil {
		slog.Error("[ir][rule] load actions error", "rule", rule.ID, "err", err)
		return
	}

	slog.Info("[ir][rule] run", "rule", rule.ID, "name", rule.Name, "actions", len(actions))
	for _, a := range actions {
		switch a.Type {
		case model.IrActionDelay:
			time.Sleep(time.Duration(min(a.DelayMs, MaxDelayMs)) * time.Millisecond)
		case model.IrActionPower:
			r.power(rule, a)
		case model.IrActionIR:
			r.sendSignal(rule, a)
		default:
			slog.Warn("[ir][rule] unknown action", "rule", rule.ID, "type", a.Type)
		}
	}
}

func (r *IrRuleRunner) power(rule model.IrRule, a model.IrRuleAction) {
	if a.TargetDeviceId == nil {
		return
	}
	target := r.states.GetById(*a.TargetDeviceId)
	if target == nil {
		slog.Warn("[ir][rule] power target missing", "rule", rule.ID, "device", *a.TargetDeviceId)
		return
	}

	var on bool
	switch a.Value {
	case model.IrPowerOn:
		on = true
	case model.IrPowerOff:
		on = false
	case model.IrPowerToggle:
		on = target.On == nil || !*target.On
	default:
		return
	}
	r.pub.OnOff(target.Device, on)
}

func (r *IrRuleRunner) sendSignal(rule model.IrRule, a model.IrRuleAction) {
	if a.SignalId == nil {
		return
	}
	var signal model.IrSignal
	if err := r.db.First(&signal, *a.SignalId).Error; err != nil {
		slog.Warn("[ir][rule] signal missing", "rule", rule.ID, "signal", *a.SignalId)
		return
	}
	target := r.states.GetById(signal.DeviceId)
	if target == nil {
		return
	}
	r.pub.SendIR(target.Device, signal.Protocol, signal.Bits, signal.Code)
}
