package internal

import (
	"log/slog"
	"slices"
	"smart-home/event"
	"smart-home/model"
	"time"

	"gorm.io/gorm"
)

// sensorTypePriority: when a device shows several kinds, the higher one wins
// (CO₂ sensor > T-H sensor > IR remote > power plug > other).
var sensorTypePriority = map[string]int{
	model.SensorTypeOther:     0,
	model.SensorTypeEnergy:    1,
	model.SensorTypeIR:        2,
	model.SensorTypeTempHumid: 3,
	model.SensorTypeCo2:       4,
}

// DetectSensorType tells the device type from its GPIO template and a sensor payload
// (either may be nil).
func DetectSensorType(gpio []int, sns *event.SensorEvent) string {
	switch {
	case sns != nil && sns.Co2 != nil:
		return model.SensorTypeCo2
	case sns != nil && sns.TempHum != nil:
		return model.SensorTypeTempHumid
	case hasIr(gpio):
		return model.SensorTypeIR
	case sns != nil && sns.Energy != nil:
		return model.SensorTypeEnergy
	}
	return model.SensorTypeOther
}

func hasIr(gpio []int) bool {
	for _, code := range gpio {
		if code > 0 && (code/32 == gpioIrSend || code/32 == gpioIrRecv) {
			return true
		}
	}
	return false
}

// HasSensorData: the payload has the data the device type stores.
func HasSensorData(sensorType string, sns *event.SensorEvent) bool {
	if sns == nil {
		return false
	}
	switch sensorType {
	case model.SensorTypeEnergy:
		return sns.Energy != nil
	case model.SensorTypeCo2:
		return sns.Co2 != nil
	case model.SensorTypeTempHumid:
		return sns.TempHum != nil
	}
	return false
}

// HasChipTemperature: the device reports the ESP32 internal temperature (SetOption146 1),
// so there is stored chip temperature to chart.
func (s *DeviceState) HasChipTemperature() bool {
	return slices.Contains(s.Device.Sensors, "ESP32")
}

// DeviceProfiles saves profile changes made by the MQTT handlers.
type DeviceProfiles struct {
	db *gorm.DB
}

func NewDeviceProfiles(db *gorm.DB) *DeviceProfiles {
	return &DeviceProfiles{db: db}
}

// Save writes the profile columns only, so it never overwrites settings edited meanwhile.
func (p *DeviceProfiles) Save(d *model.Device) {
	now := time.Now().UTC()
	d.ProfiledAt = &now
	err := p.db.Model(d).
		Select("Template", "Chip", "FirmwareVersion", "FirmwareBuiltAt", "FirmwareCore", "FirmwareSdk", "Sensors", "SensorType", "ProfiledAt").
		Updates(d).Error
	if err != nil {
		slog.Error("[device][profile] save error", "deviceId", d.ID, "err", err)
	}
}

// ApplyDetection updates the sensor type and sensor names from new evidence and reports
// whether anything changed. Normally the type only goes up in priority and names are only
// added, so a sensor missing from one message (e.g. still warming up after boot) doesn't
// demote the device. With force (manual refresh) the payload is taken as it is.
func (s *DeviceState) ApplyDetection(sns *event.SensorEvent, force bool) bool {
	d := s.Device
	var gpio []int
	if d.Template != nil {
		gpio = d.Template.Gpio
	}
	changed := false

	if sns != nil {
		names := sns.Sensors
		if !force {
			names = slices.Clone(d.Sensors)
			for _, n := range sns.Sensors {
				if !slices.Contains(names, n) {
					names = append(names, n)
				}
			}
			slices.Sort(names)
		}
		if !slices.Equal(names, d.Sensors) {
			d.Sensors = names
			changed = true
		}
	}

	detected := DetectSensorType(gpio, sns)
	if detected != d.SensorType && (force || sensorTypePriority[detected] > sensorTypePriority[d.SensorType]) {
		slog.Info("[device][profile] sensor type detected", "deviceId", d.ID, "from", d.SensorType, "to", detected)
		d.SensorType = detected
		changed = true
	}
	return changed
}
