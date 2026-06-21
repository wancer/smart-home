package web

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"smart-home/internal"
	"smart-home/mqtt"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

type DeviceTimersController struct {
	pub    *mqtt.Publisher
	states *internal.DeviceStateManager
}

func NewDeviceTimersController(pub *mqtt.Publisher, states *internal.DeviceStateManager) *DeviceTimersController {
	return &DeviceTimersController{pub: pub, states: states}
}

type TimerRequest struct {
	Enable int    `json:"enable"`
	Mode   int    `json:"mode"`
	Time   string `json:"time"`
	Window int    `json:"window"`
	Days   string `json:"days"`
	Repeat int    `json:"repeat"`
	Output int    `json:"output"`
	Action int    `json:"action"`
}

type TimerResponse struct {
	N      int    `json:"n"`
	Enable int    `json:"enable"`
	Mode   int    `json:"mode"`
	Time   string `json:"time"`
	Window int    `json:"window"`
	Days   string `json:"days"`
	Repeat int    `json:"repeat"`
	Output int    `json:"output"`
	Action int    `json:"action"`
}

func (c *DeviceTimersController) Get(w http.ResponseWriter, r *http.Request) {
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

	c.pub.GetTimers(state.Device)
	time.Sleep(3 * time.Second)

	timers := make([]TimerResponse, 16)
	for i := 0; i < 16; i++ {
		timers[i] = TimerResponse{N: i + 1, Output: 1, Time: "00:00", Days: "0000000"}
		if t := state.Config.Timers[i]; t != nil {
			timers[i].Enable = t.Enable
			timers[i].Mode = t.Mode
			timers[i].Time = t.Time
			timers[i].Window = t.Window
			timers[i].Days = t.Days
			timers[i].Repeat = t.Repeat
			timers[i].Output = t.Output
			timers[i].Action = t.Action
		}
	}

	slog.Info("[device][timers-get] success", "deviceId", deviceId)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(timers)
}

func (c *DeviceTimersController) Set(w http.ResponseWriter, r *http.Request) {
	deviceId, err := strconv.Atoi(chi.URLParam(r, "deviceId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	n, err := strconv.Atoi(chi.URLParam(r, "n"))
	if err != nil || n < 1 || n > 16 {
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
	var req TimerRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	value := fmt.Sprintf(
		`{"Enable":%d,"Mode":%d,"Time":"%s","Window":%d,"Days":"%s","Repeat":%d,"Output":%d,"Action":%d}`,
		req.Enable, req.Mode, req.Time, req.Window, req.Days, req.Repeat, req.Output, req.Action,
	)
	c.pub.SetTimer(state.Device, n, value)

	slog.Info("[device][timer-set] success", "deviceId", deviceId, "n", n)
	w.WriteHeader(http.StatusOK)
}
