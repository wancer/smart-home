package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"smart-home/event"
	"smart-home/internal"
	"time"
)

// identifyTimeout: the sensor sleeps, so a result can take 16 s or more, and one can get lost
// (MQTT disconnected when it is ready).
const identifyTimeout = 40 * time.Second

type IdentifyRequest struct {
	Seconds int `json:"seconds"` // 0..65535, 0 stops
}

type CommandResultResponse struct {
	Result string `json:"result"` // Done, Failed, Error, Busy, Unknown or Timeout
}

// Identify (POST /devices/{id}/identify) makes a Matter sensor identify itself (blink, beep) and
// waits for the bridge's reply. One command per sensor in flight. 200 Done; 409 not a Matter device
// or a command pending (ours, or Busy from the bridge); 400 bad seconds (or Error); 502 Failed /
// Unknown; 504 no reply in time.
func (c *DeviceControlController) Identify(w http.ResponseWriter, r *http.Request) {
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}
	if !state.Device.IsBridged() {
		http.Error(w, "not a Matter device", http.StatusConflict)
		return
	}
	var req IdentifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Seconds < 0 || req.Seconds > 65535 {
		http.Error(w, "seconds must be 0..65535", http.StatusBadRequest)
		return
	}

	wait, err := state.BeginCommand()
	if errors.Is(err, internal.ErrCommandPending) {
		writeCommandResult(w, http.StatusConflict, event.CommandBusy)
		return
	}
	defer state.EndCommand()

	if err := c.pub.Identify(state.Device, req.Seconds); err != nil {
		slog.Error("[device][identify] publish error", "deviceId", state.Device.ID, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	timer := time.NewTimer(identifyTimeout)
	defer timer.Stop()
	select {
	case res := <-wait:
		slog.Info("[device][identify] result", "deviceId", state.Device.ID, "result", res.Result)
		status := http.StatusBadGateway // Failed, Unknown
		switch res.Result {
		case event.CommandDone:
			status = http.StatusOK
		case event.CommandBusy:
			status = http.StatusConflict
		case event.CommandError:
			status = http.StatusBadRequest
		}
		writeCommandResult(w, status, res.Result)
	case <-timer.C:
		slog.Warn("[device][identify] no result", "deviceId", state.Device.ID)
		writeCommandResult(w, http.StatusGatewayTimeout, "Timeout")
	case <-r.Context().Done():
	}
}

func writeCommandResult(w http.ResponseWriter, status int, result string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(CommandResultResponse{Result: result})
}
