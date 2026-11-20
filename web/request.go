package web

import (
	"net/http"
	"smart-home/internal"
	"smart-home/model"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// deviceState resolves the {deviceId} URL parameter. On a non-numeric id (400) or an
// unknown device (404) it writes the error response and returns nil.
func deviceState(w http.ResponseWriter, r *http.Request, states *internal.DeviceStateManager) *internal.DeviceState {
	id, err := strconv.Atoi(chi.URLParam(r, "deviceId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return nil
	}
	state := states.GetById(uint(id))
	if state == nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return nil
	}
	return state
}

// irDevice is deviceState for IR-only endpoints: a device that isn't an IR remote is a 400.
func irDevice(w http.ResponseWriter, r *http.Request, states *internal.DeviceStateManager) *model.Device {
	state := deviceState(w, r, states)
	if state == nil {
		return nil
	}
	if state.Device.SensorType != model.SensorTypeIR {
		http.Error(w, "device is not an IR device", http.StatusBadRequest)
		return nil
	}
	return state.Device
}

// urlInt parses an integer URL parameter, writing 400 if it isn't one.
func urlInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	n, err := strconv.Atoi(chi.URLParam(r, name))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return 0, false
	}
	return n, true
}
