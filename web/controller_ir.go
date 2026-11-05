package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"smart-home/internal"
	"smart-home/model"
	"smart-home/mqtt"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

var irCodePattern = regexp.MustCompile(`^0[xX][0-9a-fA-F]{1,16}$`)

type IrController struct {
	db        *gorm.DB
	states    *internal.DeviceStateManager
	publisher *mqtt.Publisher
}

func NewIrController(db *gorm.DB, states *internal.DeviceStateManager, publisher *mqtt.Publisher) *IrController {
	return &IrController{db: db, states: states, publisher: publisher}
}

type IrSignalRequest struct {
	Group    string `json:"group"`
	Name     string `json:"name"`
	Code     string `json:"code"`
	Protocol string `json:"protocol"`
	Bits     int    `json:"bits"`
}

type IrSignalResponse struct {
	ID       uint   `json:"id"`
	Group    string `json:"group"`
	Name     string `json:"name"`
	Code     string `json:"code"`
	Protocol string `json:"protocol"`
	Bits     int    `json:"bits"`
}

type IrGroupResponse struct {
	Group   string             `json:"group"`
	Signals []IrSignalResponse `json:"signals"`
}

func newIrSignalResponse(s *model.IrSignal) IrSignalResponse {
	return IrSignalResponse{ID: s.ID, Group: s.Group, Name: s.Name, Code: s.Code, Protocol: s.Protocol, Bits: s.Bits}
}

// normalize validates the request and fills defaults. Returns false if invalid.
func (req *IrSignalRequest) normalize() bool {
	req.Group = strings.TrimSpace(req.Group)
	req.Name = strings.TrimSpace(req.Name)
	req.Code = strings.TrimSpace(req.Code)
	if req.Group == "" || req.Name == "" || !irCodePattern.MatchString(req.Code) {
		return false
	}
	// Canonical form: lower 0x prefix, upper hex digits, e.g. 0x00F740BF
	req.Code = "0x" + strings.ToUpper(req.Code[2:])
	if req.Protocol == "" {
		req.Protocol = "NEC"
	}
	if req.Bits == 0 {
		req.Bits = 32
	}
	return req.Bits > 0 && req.Bits <= 64
}

// irDevice resolves {deviceId} and ensures it is an IR device.
func (c *IrController) irDevice(w http.ResponseWriter, r *http.Request) *model.Device {
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

func (c *IrController) loadSignal(w http.ResponseWriter, r *http.Request, device *model.Device) *model.IrSignal {
	signalId, err := strconv.Atoi(chi.URLParam(r, "signalId"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return nil
	}
	var signal model.IrSignal
	if err := c.db.WithContext(r.Context()).Where("id = ? AND device_id = ?", signalId, device.ID).First(&signal).Error; err != nil {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return nil
	}
	return &signal
}

// Get returns all signals of the device grouped by group name.
func (c *IrController) Get(w http.ResponseWriter, r *http.Request) {
	device := c.irDevice(w, r)
	if device == nil {
		return
	}

	var signals []model.IrSignal
	if err := c.db.WithContext(r.Context()).
		Where("device_id = ?", device.ID).
		Order("group_name, position, id").
		Find(&signals).Error; err != nil {
		slog.Error("[ir][get] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	groups := []IrGroupResponse{}
	index := map[string]int{}
	for i := range signals {
		g := signals[i].Group
		pos, ok := index[g]
		if !ok {
			pos = len(groups)
			index[g] = pos
			groups = append(groups, IrGroupResponse{Group: g, Signals: []IrSignalResponse{}})
		}
		groups[pos].Signals = append(groups[pos].Signals, newIrSignalResponse(&signals[i]))
	}

	json.NewEncoder(w).Encode(groups)
}

func (c *IrController) Create(w http.ResponseWriter, r *http.Request) {
	device := c.irDevice(w, r)
	if device == nil {
		return
	}

	var req IrSignalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !req.normalize() {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	var maxPos *int
	c.db.WithContext(r.Context()).Model(&model.IrSignal{}).
		Where("device_id = ? AND group_name = ?", device.ID, req.Group).
		Select("MAX(position)").Scan(&maxPos)
	position := 0
	if maxPos != nil {
		position = *maxPos + 1
	}

	signal := &model.IrSignal{
		DeviceId: device.ID, Group: req.Group, Name: req.Name,
		Code: req.Code, Protocol: req.Protocol, Bits: req.Bits, Position: position,
	}
	if err := c.db.WithContext(r.Context()).Create(signal).Error; err != nil {
		slog.Error("[ir][create] error", "err", err)
		http.Error(w, "signal already exists or could not be saved", http.StatusConflict)
		return
	}

	slog.Info("[ir][create] success", "device", device.ID, "id", signal.ID)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newIrSignalResponse(signal))
}

func (c *IrController) Update(w http.ResponseWriter, r *http.Request) {
	device := c.irDevice(w, r)
	if device == nil {
		return
	}
	signal := c.loadSignal(w, r, device)
	if signal == nil {
		return
	}

	var req IrSignalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !req.normalize() {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	signal.Group, signal.Name, signal.Code = req.Group, req.Name, req.Code
	signal.Protocol, signal.Bits = req.Protocol, req.Bits
	if err := c.db.WithContext(r.Context()).Save(signal).Error; err != nil {
		slog.Error("[ir][update] error", "err", err)
		http.Error(w, "signal already exists or could not be saved", http.StatusConflict)
		return
	}

	slog.Info("[ir][update] success", "device", device.ID, "id", signal.ID)
	json.NewEncoder(w).Encode(newIrSignalResponse(signal))
}

func (c *IrController) Delete(w http.ResponseWriter, r *http.Request) {
	device := c.irDevice(w, r)
	if device == nil {
		return
	}
	signal := c.loadSignal(w, r, device)
	if signal == nil {
		return
	}

	if err := c.db.WithContext(r.Context()).Delete(signal).Error; err != nil {
		slog.Error("[ir][delete] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	slog.Info("[ir][delete] success", "device", device.ID, "id", signal.ID)
	w.WriteHeader(http.StatusNoContent)
}

// Send transmits a stored signal through the device.
func (c *IrController) Send(w http.ResponseWriter, r *http.Request) {
	device := c.irDevice(w, r)
	if device == nil {
		return
	}
	signal := c.loadSignal(w, r, device)
	if signal == nil {
		return
	}

	c.publisher.SendIR(device, signal.Protocol, signal.Bits, signal.Code)
	slog.Info("[ir][send] success", "device", device.ID, "id", signal.ID, "code", signal.Code)
	w.WriteHeader(http.StatusNoContent)
}
