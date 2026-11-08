package web

import (
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"smart-home/automation"
	"smart-home/internal"
	"smart-home/model"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

type NotificationsController struct {
	db       *gorm.DB
	states   *internal.DeviceStateManager
	notifier *automation.Notifier
	telegram *internal.TelegramNotifier
}

func NewNotificationsController(
	db *gorm.DB,
	states *internal.DeviceStateManager,
	notifier *automation.Notifier,
	telegram *internal.TelegramNotifier,
) *NotificationsController {
	return &NotificationsController{db: db, states: states, notifier: notifier, telegram: telegram}
}

type NotifyRuleRequest struct {
	Kind           string  `json:"kind"`             // offline | online | above | below
	Metric         string  `json:"metric,omitempty"` // above/below: power | current | voltage | co2 | temperature | humidity
	Threshold      float64 `json:"threshold,omitempty"`
	Duration       int     `json:"duration"` // seconds
	NotifyRecovery bool    `json:"notifyRecovery,omitempty"`
	Message        string  `json:"message,omitempty"`
	Enabled        bool    `json:"enabled"`
}

type NotifyRuleResponse struct {
	ID uint `json:"id"`
	NotifyRuleRequest
	Active bool `json:"active"` // fired and not recovered yet
}

type NotificationsResponse struct {
	TelegramConfigured bool                 `json:"telegramConfigured"`
	Metrics            []string             `json:"metrics"` // metrics available for this device
	Rules              []NotifyRuleResponse `json:"rules"`
}

func (c *NotificationsController) device(w http.ResponseWriter, r *http.Request) *model.Device {
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
	return state.Device
}

// validate normalises the request and returns a human-readable problem, or "".
func validateNotifyRule(device *model.Device, req *NotifyRuleRequest) string {
	req.Message = strings.TrimSpace(req.Message)
	if utf8.RuneCountInString(req.Message) > 255 {
		return "message must be at most 255 characters"
	}
	if req.Duration < 0 || req.Duration > int(automation.MaxNotifyDuration.Seconds()) {
		return "duration must be between 0 and " + strconv.Itoa(int(automation.MaxNotifyDuration.Seconds())) + " seconds"
	}

	switch req.Kind {
	case model.NotifyOffline, model.NotifyOnline:
		req.Metric, req.Threshold, req.NotifyRecovery = "", 0, false
	case model.NotifyAbove, model.NotifyBelow:
		metrics := model.MetricsBySensorType[device.SensorType]
		if !slices.Contains(metrics, req.Metric) {
			if len(metrics) == 0 {
				return "this device has no sensors for threshold notifications"
			}
			return "metric must be one of: " + strings.Join(metrics, ", ")
		}
		if math.IsNaN(req.Threshold) || math.IsInf(req.Threshold, 0) {
			return "threshold must be a number"
		}
	default:
		return "kind must be one of: offline, online, above, below"
	}
	return ""
}

func (c *NotificationsController) toResponse(rule *model.NotifyRule) NotifyRuleResponse {
	return NotifyRuleResponse{
		ID: rule.ID,
		NotifyRuleRequest: NotifyRuleRequest{
			Kind: rule.Kind, Metric: rule.Metric, Threshold: rule.Threshold, Duration: rule.DurationSec,
			NotifyRecovery: rule.NotifyRecovery, Message: rule.Message, Enabled: rule.Enabled,
		},
		Active: rule.Enabled && c.notifier.Active(rule.ID),
	}
}

func (c *NotificationsController) apply(rule *model.NotifyRule, req *NotifyRuleRequest) {
	rule.Kind, rule.Metric, rule.Threshold, rule.DurationSec = req.Kind, req.Metric, req.Threshold, req.Duration
	rule.NotifyRecovery, rule.Message, rule.Enabled = req.NotifyRecovery, req.Message, req.Enabled
}

func (c *NotificationsController) reload() {
	if err := c.notifier.Reload(); err != nil {
		slog.Error("[notify-rule] reload error", "err", err)
	}
}

func (c *NotificationsController) Get(w http.ResponseWriter, r *http.Request) {
	device := c.device(w, r)
	if device == nil {
		return
	}

	var rules []model.NotifyRule
	if err := c.db.WithContext(r.Context()).Where("device_id = ?", device.ID).Order("id").Find(&rules).Error; err != nil {
		slog.Error("[notify-rule][get] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	out := NotificationsResponse{
		TelegramConfigured: c.telegram.Enabled(),
		Metrics:            model.MetricsBySensorType[device.SensorType],
		Rules:              make([]NotifyRuleResponse, len(rules)),
	}
	if out.Metrics == nil {
		out.Metrics = []string{}
	}
	for i := range rules {
		out.Rules[i] = c.toResponse(&rules[i])
	}
	json.NewEncoder(w).Encode(out)
}

func (c *NotificationsController) Create(w http.ResponseWriter, r *http.Request) {
	device := c.device(w, r)
	if device == nil {
		return
	}
	var req NotifyRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if msg := validateNotifyRule(device, &req); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	rule := &model.NotifyRule{DeviceId: device.ID}
	c.apply(rule, &req)
	// Select("*") so a false Enabled is written instead of the column default
	if err := c.db.WithContext(r.Context()).Select("*").Create(rule).Error; err != nil {
		slog.Error("[notify-rule][create] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	c.reload()

	slog.Info("[notify-rule][create] success", "device", device.ID, "id", rule.ID)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(c.toResponse(rule))
}

func (c *NotificationsController) Update(w http.ResponseWriter, r *http.Request) {
	device := c.device(w, r)
	if device == nil {
		return
	}
	ruleId, err := strconv.Atoi(chi.URLParam(r, "ruleId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	var rule model.NotifyRule
	if err := c.db.WithContext(r.Context()).Where("id = ? AND device_id = ?", ruleId, device.ID).First(&rule).Error; err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	var req NotifyRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if msg := validateNotifyRule(device, &req); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	c.apply(&rule, &req)
	if err := c.db.WithContext(r.Context()).Save(&rule).Error; err != nil {
		slog.Error("[notify-rule][update] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	c.reload()

	slog.Info("[notify-rule][update] success", "device", device.ID, "id", rule.ID)
	json.NewEncoder(w).Encode(c.toResponse(&rule))
}

func (c *NotificationsController) Delete(w http.ResponseWriter, r *http.Request) {
	device := c.device(w, r)
	if device == nil {
		return
	}
	ruleId, err := strconv.Atoi(chi.URLParam(r, "ruleId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	res := c.db.WithContext(r.Context()).Where("id = ? AND device_id = ?", ruleId, device.ID).Delete(&model.NotifyRule{})
	if res.Error != nil {
		slog.Error("[notify-rule][delete] error", "err", res.Error)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if res.RowsAffected == 0 {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	c.reload()

	slog.Info("[notify-rule][delete] success", "device", device.ID, "id", ruleId)
	w.WriteHeader(http.StatusNoContent)
}

// Test sends a message right away, to check the bot token and chat id.
func (c *NotificationsController) Test(w http.ResponseWriter, r *http.Request) {
	if !c.telegram.Enabled() {
		http.Error(w, "set TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID", http.StatusServiceUnavailable)
		return
	}
	if err := c.telegram.Send("🔔 Smart home: test notification"); err != nil {
		slog.Warn("[notify][test] failed", "err", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
