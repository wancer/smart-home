package model

import (
	"fmt"
	"time"
)

// MqConfig is an MQ-xxx gas sensor (MQ-135, ...) on a Tasmota analog input ("ADC Input"):
// the backend turns the raw ADC value into the sensor resistance Rs and the gas ratio
// (internal.DeviceState.ComputeMq). Calibration (R0) lives here too, not on the device.
type MqConfig struct {
	Enabled bool   `json:"enabled"`
	Input   string `json:"input"` // key in the ANALOG object, e.g. A1

	LoadOhm     float32 `json:"loadOhm"`     // load resistor RL on the module, Ω
	AdcVolts    float32 `json:"adcVolts"`    // voltage at full scale of the ADC, V
	AdcMax      float32 `json:"adcMax"`      // raw value at full scale
	Divider     float32 `json:"divider"`     // GPIO voltage / module output; 1 = no divider (a series resistor isn't one)
	SupplyVolts float32 `json:"supplyVolts"` // module supply, V, when no DC monitor (INA219) reports it

	R0        float32    `json:"r0"`        // Rs in the calibration air (Ratio 1.0), Ω; 0 = not calibrated
	PrevR0    *float32   `json:"prevR0"`    // R0 before the last change; nil = none yet
	ChangedAt *time.Time `json:"changedAt"` // when R0 last changed
}

// DefaultMqConfig: an FC-22 style module (1 kΩ load) on an ESP32 ADC (12 bit, 3.3 V), 5 V supply.
func DefaultMqConfig() MqConfig {
	return MqConfig{
		Input:       "A1",
		LoadOhm:     1000,
		AdcVolts:    3.3,
		AdcMax:      4095,
		Divider:     1,
		SupplyVolts: 5,
	}
}

// ValidateMqSettings checks the hardware settings (not the calibration).
func ValidateMqSettings(c *MqConfig) error {
	switch {
	case c.Input == "" || len(c.Input) > 32:
		return fmt.Errorf("input must be an ANALOG key, e.g. A1")
	case c.LoadOhm < 10 || c.LoadOhm > 1_000_000:
		return fmt.Errorf("loadOhm must be 10..1000000")
	case c.AdcVolts < 0.5 || c.AdcVolts > 6:
		return fmt.Errorf("adcVolts must be 0.5..6")
	case c.AdcMax < 255 || c.AdcMax > 65535:
		return fmt.Errorf("adcMax must be 255..65535")
	case c.Divider <= 0 || c.Divider > 1:
		return fmt.Errorf("divider must be 0..1 (1 = none)")
	case c.SupplyVolts < 1 || c.SupplyVolts > 24:
		return fmt.Errorf("supplyVolts must be 1..24")
	}
	return nil
}
