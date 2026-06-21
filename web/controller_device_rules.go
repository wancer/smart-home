package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"smart-home/mqtt"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

type DeviceRulesController struct {
	pub    *mqtt.Publisher
	states *internal.DeviceStateManager
}

func NewDeviceRulesController(pub *mqtt.Publisher, states *internal.DeviceStateManager) *DeviceRulesController {
	return &DeviceRulesController{pub: pub, states: states}
}

type RuleRequest struct {
	Rules string `json:"rules"`
	State int    `json:"state"`
	Once  int    `json:"once"`
}

type RuleResponse struct {
	N     int    `json:"n"`
	State int    `json:"state"`
	Once  int    `json:"once"`
	Rules string `json:"rules"`
	Free  int    `json:"free"`
}

func (c *DeviceRulesController) Get(w http.ResponseWriter, r *http.Request) {
	deviceId, err := strconv.Atoi(chi.URLParam(r, "deviceId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	state := c.states.GetById(uint(deviceId))
	if state == nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	c.pub.GetRules(state.Device)
	time.Sleep(3 * time.Second)

	rules := make([]RuleResponse, 3)
	for i := 0; i < 3; i++ {
		rules[i] = RuleResponse{N: i + 1}
		if rc := state.Config.Rules[i]; rc != nil {
			rules[i].State = rc.State
			rules[i].Once = rc.Once
			rules[i].Rules = rc.Rules
			rules[i].Free = rc.Free
		}
	}

	slog.Info("[device][rules-get] success", "deviceId", deviceId)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(rules)
}

func (c *DeviceRulesController) Set(w http.ResponseWriter, r *http.Request) {
	deviceId, err := strconv.Atoi(chi.URLParam(r, "deviceId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	n, err := strconv.Atoi(chi.URLParam(r, "n"))
	if err != nil || n < 1 || n > 3 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	state := c.states.GetById(uint(deviceId))
	if state == nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	var req RuleRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	// Set rule text first when provided (non-empty)
	if req.Rules != "" {
		c.pub.SetRule(state.Device, n, req.Rules)
		time.Sleep(500 * time.Millisecond)
	}

	// Set enabled state:
	// 0 = disabled, 1 = enabled (repeat), 2 = enabled once
	stateCmd := req.State
	if req.State == 1 && req.Once == 1 {
		stateCmd = 2
	}
	c.pub.SetRule(state.Device, n, strconv.Itoa(stateCmd))

	slog.Info("[device][rule-set] success", "deviceId", deviceId, "n", n)
	w.WriteHeader(http.StatusOK)
}
