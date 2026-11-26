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
// (multi-sensor > leak sensor > contact sensor > CO₂ sensor > T-H sensor > IR remote > power plug > light > switch > other).
var sensorTypePriority = map[string]int{
	model.SensorTypeMulti:     9,
	model.SensorTypeLeak:      8,
	model.SensorTypeContact:   7,
	model.SensorTypeOther:     0,
	model.SensorTypeSwitch:    1,
	model.SensorTypeLight:     2,
	model.SensorTypeEnergy:    3,
	model.SensorTypeIR:        4,
	model.SensorTypeTempHumid: 5,
	model.SensorTypeCo2:       6,
}

// DetectSensorType tells the device type from its GPIO template and a sensor payload
// (either may be nil).
func DetectSensorType(gpio []int, sns *event.SensorEvent) string {
	switch {
	case isMultiSensor(sns):
		return model.SensorTypeMulti
	case sns != nil && sns.Leak != nil:
		return model.SensorTypeLeak
	case sns != nil && sns.Contact != nil:
		return model.SensorTypeContact
	case sns != nil && sns.Co2 != nil:
		return model.SensorTypeCo2
	case sns != nil && (sns.TempHum != nil || sns.Thermo != nil):
		return model.SensorTypeTempHumid
	case hasIr(gpio):
		return model.SensorTypeIR
	case sns != nil && sns.Energy != nil:
		return model.SensorTypeEnergy
	}
	switch hw := DetectLedHardware(gpio); {
	case hw.Light != LightNone:
		return model.SensorTypeLight
	case hw.Relays > 0:
		return model.SensorTypeSwitch
	}
	return model.SensorTypeOther
}

// isMultiSensor: a sensor only the multi type shows (pressure, DC monitor, MQ gas sensor, light or
// colour sensor), or an energy meter together with a climate sensor (CO2, T-H or a thermometer).
func isMultiSensor(sns *event.SensorEvent) bool {
	if sns == nil {
		return false
	}
	return sns.Baro != nil || sns.Dc != nil || sns.Mq != nil || sns.Light != nil || sns.Color != nil ||
		(sns.Energy != nil && (sns.Co2 != nil || sns.TempHum != nil || sns.Thermo != nil))
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
	case model.SensorTypeLeak:
		return sns.Leak != nil
	case model.SensorTypeContact:
		return sns.Contact != nil
	case model.SensorTypeCo2:
		return sns.Co2 != nil
	case model.SensorTypeTempHumid:
		return sns.TempHum != nil || sns.Thermo != nil
	case model.SensorTypeMulti:
		return sns.Energy != nil || sns.Co2 != nil || sns.TempHum != nil || sns.Thermo != nil || sns.Baro != nil || sns.Dc != nil || sns.Mq != nil ||
			sns.Light != nil || sns.Color != nil || sns.Leak != nil || sns.Contact != nil
	}
	return false
}

// HasEnergyMeter: the device has an AC energy meter (power/voltage calibration, daily kWh).
func (s *DeviceState) HasEnergyMeter() bool {
	return s.Device.SensorType == model.SensorTypeEnergy || slices.Contains(s.Device.Sensors, "ENERGY")
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

// SavePlatform writes the platform column only (platform auto-detection).
func (p *DeviceProfiles) SavePlatform(d *model.Device) {
	if err := p.db.Model(d).Select("Platform").Updates(d).Error; err != nil {
		slog.Error("[device][platform] save error", "deviceId", d.ID, "err", err)
	}
}

// DetectPlatform switches a Tasmota device to the Matter platform when its telemetry has the
// top-level "Available" of a bridged end device (Tasmota never sends it) and nothing was ever read
// from Tasmota firmware (template, version). Reports whether it changed; the caller dispatches
// event.PlatformChanged so the MQTT subscriptions follow (the bridge is DefaultMatterBridge).
func (s *DeviceState) DetectPlatform(sns *event.SensorEvent) bool {
	d := s.Device
	if sns == nil || sns.Available == nil || !d.IsTasmota() || d.Template != nil || d.FirmwareVersion != "" {
		return false
	}
	slog.Info("[device][profile] platform detected", "deviceId", d.ID, "from", d.Platform, "to", model.PlatformMatter)
	d.Platform = model.PlatformMatter
	return true
}

// BareTelemetryIsLive: telemetry without the device type's sensor data still sets the live
// values and keeps the device alive (IR remotes, unrecognised devices; a bridged device of
// unknown type, which has no LWT of its own).
func BareTelemetryIsLive(d *model.Device) bool {
	return d.AvailabilityFromLwt() || (d.IsBridged() && d.SensorType == model.SensorTypeOther)
}

// SaveMq writes the MQ sensor column only (settings and calibration).
func (p *DeviceProfiles) SaveMq(d *model.Device) error {
	if err := p.db.Model(d).Select("Mq").Updates(d).Error; err != nil {
		slog.Error("[device][mq] save error", "deviceId", d.ID, "err", err)
		return err
	}
	return nil
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
