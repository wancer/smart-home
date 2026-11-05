package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"smart-home/automation"
	"smart-home/internal"
	"smart-home/model"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type IrRulesController struct {
	db     *gorm.DB
	states *internal.DeviceStateManager
	runner *automation.IrRuleRunner
}

func NewIrRulesController(db *gorm.DB, states *internal.DeviceStateManager, runner *automation.IrRuleRunner) *IrRulesController {
	return &IrRulesController{db: db, states: states, runner: runner}
}

type IrActionDto struct {
	Type           string `json:"type"` // power | ir | delay
	TargetDeviceId *uint  `json:"targetDeviceId,omitempty"`
	Value          string `json:"value,omitempty"` // on | off | toggle
	SignalId       *uint  `json:"signalId,omitempty"`
	DelayMs        int    `json:"delayMs,omitempty"`
}

type IrRuleRequest struct {
	Name    string        `json:"name"`
	Code    string        `json:"code"`
	Enabled bool          `json:"enabled"`
	Actions []IrActionDto `json:"actions"`
}

type IrRuleResponse struct {
	ID      uint          `json:"id"`
	Name    string        `json:"name"`
	Code    string        `json:"code"`
	Enabled bool          `json:"enabled"`
	Actions []IrActionDto `json:"actions"`
}

func (c *IrRulesController) irDevice(w http.ResponseWriter, r *http.Request) *model.Device {
	deviceId, err := strconv.Atoi(chi.URLParam(r, "deviceId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return nil
	}
	state := c.states.GetById(uint(deviceId))
	if state == nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return nil
	}
	if state.Device.SensorType != model.SensorTypeIR {
		http.Error(w, "device is not an IR device", http.StatusBadRequest)
		return nil
	}
	return state.Device
}

// validate checks the request and returns a human-readable problem, or "".
func (c *IrRulesController) validate(r *http.Request, req *IrRuleRequest) string {
	req.Name = strings.TrimSpace(req.Name)
	req.Code = strings.TrimSpace(req.Code)
	if req.Name == "" || !irCodePattern.MatchString(req.Code) {
		return "name and a code like 0x00F740BF are required"
	}
	req.Code = "0x" + strings.ToUpper(req.Code[2:])
	if len(req.Actions) == 0 || len(req.Actions) > automation.MaxRuleSteps {
		return "a rule needs between 1 and " + strconv.Itoa(automation.MaxRuleSteps) + " actions"
	}

	for i := range req.Actions {
		a := &req.Actions[i]
		switch a.Type {
		case model.IrActionPower:
			if a.Value != model.IrPowerOn && a.Value != model.IrPowerOff && a.Value != model.IrPowerToggle {
				return "power action value must be on, off or toggle"
			}
			if a.TargetDeviceId == nil {
				return "power action needs targetDeviceId"
			}
			t := c.states.GetById(*a.TargetDeviceId)
			if t == nil || t.Device.SensorType == model.SensorTypeIR {
				return "power action target must be an existing non-IR device"
			}
			a.SignalId, a.DelayMs = nil, 0
		case model.IrActionIR:
			if a.SignalId == nil {
				return "ir action needs signalId"
			}
			var n int64
			c.db.WithContext(r.Context()).Model(&model.IrSignal{}).Where("id = ?", *a.SignalId).Count(&n)
			if n == 0 {
				return "ir action signal does not exist"
			}
			a.TargetDeviceId, a.Value, a.DelayMs = nil, "", 0
		case model.IrActionDelay:
			if a.DelayMs < 1 || a.DelayMs > automation.MaxDelayMs {
				return "delay must be between 1 and " + strconv.Itoa(automation.MaxDelayMs) + " ms"
			}
			a.TargetDeviceId, a.Value, a.SignalId = nil, "", nil
		default:
			return "unknown action type " + a.Type
		}
	}
	return ""
}

func toActionRows(ruleId uint, actions []IrActionDto) []model.IrRuleAction {
	rows := make([]model.IrRuleAction, len(actions))
	for i, a := range actions {
		rows[i] = model.IrRuleAction{
			RuleId: ruleId, Position: i, Type: a.Type,
			TargetDeviceId: a.TargetDeviceId, Value: a.Value, SignalId: a.SignalId, DelayMs: a.DelayMs,
		}
	}
	return rows
}

func toRuleResponse(rule *model.IrRule, actions []model.IrRuleAction) IrRuleResponse {
	dtos := make([]IrActionDto, len(actions))
	for i, a := range actions {
		dtos[i] = IrActionDto{Type: a.Type, TargetDeviceId: a.TargetDeviceId, Value: a.Value, SignalId: a.SignalId, DelayMs: a.DelayMs}
	}
	return IrRuleResponse{ID: rule.ID, Name: rule.Name, Code: rule.Code, Enabled: rule.Enabled, Actions: dtos}
}

func (c *IrRulesController) Get(w http.ResponseWriter, r *http.Request) {
	device := c.irDevice(w, r)
	if device == nil {
		return
	}

	var rules []model.IrRule
	if err := c.db.WithContext(r.Context()).Where("device_id = ?", device.ID).Order("id").Find(&rules).Error; err != nil {
		slog.Error("[ir-rule][get] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	var actions []model.IrRuleAction
	ids := make([]uint, len(rules))
	for i, rule := range rules {
		ids[i] = rule.ID
	}
	if len(ids) > 0 {
		c.db.WithContext(r.Context()).Where("rule_id IN ?", ids).Order("rule_id, position").Find(&actions)
	}
	byRule := map[uint][]model.IrRuleAction{}
	for _, a := range actions {
		byRule[a.RuleId] = append(byRule[a.RuleId], a)
	}

	out := make([]IrRuleResponse, len(rules))
	for i := range rules {
		out[i] = toRuleResponse(&rules[i], byRule[rules[i].ID])
	}
	json.NewEncoder(w).Encode(out)
}

func (c *IrRulesController) Create(w http.ResponseWriter, r *http.Request) {
	device := c.irDevice(w, r)
	if device == nil {
		return
	}
	var req IrRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if msg := c.validate(r, &req); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	rule := &model.IrRule{DeviceId: device.ID, Name: req.Name, Code: req.Code, Enabled: req.Enabled}
	rows := toActionRows(0, req.Actions)
	err := c.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(rule).Error; err != nil {
			return err
		}
		for i := range rows {
			rows[i].RuleId = rule.ID
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		slog.Error("[ir-rule][create] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	slog.Info("[ir-rule][create] success", "device", device.ID, "id", rule.ID)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toRuleResponse(rule, rows))
}

func (c *IrRulesController) Update(w http.ResponseWriter, r *http.Request) {
	device := c.irDevice(w, r)
	if device == nil {
		return
	}
	ruleId, err := strconv.Atoi(chi.URLParam(r, "ruleId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	var rule model.IrRule
	if err := c.db.WithContext(r.Context()).Where("id = ? AND device_id = ?", ruleId, device.ID).First(&rule).Error; err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	var req IrRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if msg := c.validate(r, &req); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	rule.Name, rule.Code, rule.Enabled = req.Name, req.Code, req.Enabled
	rows := toActionRows(rule.ID, req.Actions)
	err = c.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&rule).Error; err != nil {
			return err
		}
		if err := tx.Where("rule_id = ?", rule.ID).Delete(&model.IrRuleAction{}).Error; err != nil {
			return err
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		slog.Error("[ir-rule][update] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	slog.Info("[ir-rule][update] success", "device", device.ID, "id", rule.ID)
	json.NewEncoder(w).Encode(toRuleResponse(&rule, rows))
}

func (c *IrRulesController) Delete(w http.ResponseWriter, r *http.Request) {
	device := c.irDevice(w, r)
	if device == nil {
		return
	}
	ruleId, err := strconv.Atoi(chi.URLParam(r, "ruleId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	res := c.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		del := tx.Where("id = ? AND device_id = ?", ruleId, device.ID).Delete(&model.IrRule{})
		if del.Error != nil {
			return del.Error
		}
		if del.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.Where("rule_id = ?", ruleId).Delete(&model.IrRuleAction{}).Error
	})
	if res == gorm.ErrRecordNotFound {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	if res != nil {
		slog.Error("[ir-rule][delete] error", "err", res)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	slog.Info("[ir-rule][delete] success", "device", device.ID, "id", ruleId)
	w.WriteHeader(http.StatusNoContent)
}

// Received returns the latest codes seen by the device's receiver, newest first.
// Press a remote button, then poll this to pick the code for a signal or rule.
func (c *IrRulesController) Received(w http.ResponseWriter, r *http.Request) {
	device := c.irDevice(w, r)
	if device == nil {
		return
	}
	json.NewEncoder(w).Encode(c.runner.Received(device.ID))
}
