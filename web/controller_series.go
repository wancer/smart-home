package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"
)

// SeriesUpdateRequest sets the user-editable fields of a series.
type SeriesUpdateRequest struct {
	// Label replaces the derived name (kind, plus the source when two series share a kind);
	// null or blank = derived.
	Label  *string `json:"label"`
	Hidden bool    `json:"hidden"`
}

// UpdateSeries sets a series' label and hidden flag: PUT /devices/{deviceId}/series/{seriesId}.
// 404 for a series of another device. The new metadata goes out on the WS `series` channel.
func (c *DevicesController) UpdateSeries(w http.ResponseWriter, r *http.Request) {
	state := deviceState(w, r, c.states)
	if state == nil {
		return
	}
	seriesId, ok := urlInt(w, r, "seriesId")
	if !ok {
		return
	}
	series, found := c.storage.Series(uint(seriesId))
	if !found || series.DeviceId != state.Device.ID {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	var req SeriesUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if req.Label != nil {
		label := strings.TrimSpace(*req.Label)
		if utf8.RuneCountInString(label) > 64 {
			http.Error(w, "label must be at most 64 characters", http.StatusBadRequest)
			return
		}
		req.Label = &label
		if label == "" {
			req.Label = nil
		}
	}

	if err := c.storage.UpdateSeries(r.Context(), series.Id, req.Label, req.Hidden); err != nil {
		slog.Error("[series][update] error", "err", err, "id", series.Id)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	updated, _ := c.storage.Series(series.Id)
	slog.Info("[series][update] success", "device", state.Device.ID, "id", series.Id)
	json.NewEncoder(w).Encode(newSeriesResponse(updated))
}
