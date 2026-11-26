package model

import "strings"

// Kinds of sensor series: the physical quantity a series measures. A kind gives the unit and the
// display precision (mirrored in the clients, which add colour, chart group, ...). A series is one
// device × sensor (source) × field; its kind comes from SeriesKind.
const (
	KindPower           = "power"
	KindApparentPower   = "apparent_power"
	KindReactivePower   = "reactive_power"
	KindPowerFactor     = "power_factor"
	KindVoltage         = "voltage"
	KindCurrent         = "current"
	KindDcVoltage       = "dc_voltage"
	KindDcCurrent       = "dc_current"
	KindDcPower         = "dc_power"
	KindDcBusVoltage    = "dc_bus_voltage"   // INA219/INA226 bus voltage (supply side)
	KindDcShuntVoltage  = "dc_shunt_voltage" // INA219/INA226 voltage across the shunt (mV)
	KindTemperature     = "temperature"
	KindChipTemperature = "chip_temperature"
	KindHumidity        = "humidity"
	KindDewPoint        = "dew_point"
	KindCo2             = "co2"
	KindPressure        = "pressure"
	KindIlluminance     = "illuminance"
	KindColorTemp       = "color_temp"
	KindColorRed        = "color_red" // colour sensor channels (TCS34725): % of full scale
	KindColorGreen      = "color_green"
	KindColorBlue       = "color_blue"
	KindColorClear      = "color_clear" // the unfiltered channel
	KindGasRatio        = "gas_ratio"
	KindLeak            = "leak"    // water leak sensor: 1 = leak, 0 = dry
	KindContact         = "contact" // contact sensor: 1 = closed, 0 = open
	KindBattery         = "battery"
	KindBatteryVoltage  = "battery_voltage"
	KindRaw             = "raw"
)

// Series sources with a meaning of their own (Tasmota sensor object names, plus the computed ones).
const (
	SourceEnergy = "ENERGY" // AC energy meter: its Voltage/Current/Power are AC, any other source's are DC
	SourceChip   = "ESP32"  // ESP32 internal temperature (SetOption146 1)
	SourceMq     = "MQ"     // MQ-xxx gas sensor, computed by the backend (Ratio, Rs)
	SourceLegacy = "legacy" // migrated data whose sensor can't be told
	// SourceDevice holds the top-level fields of a payload: a bridged (Matter) end device sends
	// its values flat, not in a sensor object ({"Leak":false,"Battery":98,...}).
	SourceDevice = "Device"
)

type KindInfo struct {
	Kind      string
	Unit      string // "" = unitless
	Precision int    // decimals shown
}

// Kinds is every kind with its unit and display precision.
var Kinds = map[string]KindInfo{
	KindPower:           {KindPower, "W", 0},
	KindApparentPower:   {KindApparentPower, "VA", 0},
	KindReactivePower:   {KindReactivePower, "var", 0},
	KindPowerFactor:     {KindPowerFactor, "", 2},
	KindVoltage:         {KindVoltage, "V", 0},
	KindCurrent:         {KindCurrent, "A", 3},
	KindDcVoltage:       {KindDcVoltage, "V", 2},
	KindDcCurrent:       {KindDcCurrent, "A", 3},
	KindDcPower:         {KindDcPower, "W", 2},
	KindDcBusVoltage:    {KindDcBusVoltage, "V", 2},
	KindDcShuntVoltage:  {KindDcShuntVoltage, "mV", 2},
	KindTemperature:     {KindTemperature, "°C", 1},
	KindChipTemperature: {KindChipTemperature, "°C", 1},
	KindHumidity:        {KindHumidity, "%", 1},
	KindDewPoint:        {KindDewPoint, "°C", 1},
	KindCo2:             {KindCo2, "ppm", 0},
	KindPressure:        {KindPressure, "hPa", 1},
	KindIlluminance:     {KindIlluminance, "lx", 0},
	KindColorTemp:       {KindColorTemp, "K", 0},
	KindColorRed:        {KindColorRed, "%", 1},
	KindColorGreen:      {KindColorGreen, "%", 1},
	KindColorBlue:       {KindColorBlue, "%", 1},
	KindColorClear:      {KindColorClear, "%", 1},
	KindGasRatio:        {KindGasRatio, "", 2},
	KindLeak:            {KindLeak, "", 0},
	KindContact:         {KindContact, "", 0},
	KindBattery:         {KindBattery, "%", 0},
	KindBatteryVoltage:  {KindBatteryVoltage, "V", 2},
	KindRaw:             {KindRaw, "", 2},
}

// liveOnlyKinds are shown as live values only (tiles, dashboard, notifications): their series
// exist, but no history is stored (derived or rarely useful values: VA, var, cos φ, dew point,
// INA bus/shunt voltage, colour channels), and Storage.PruneValues deletes any stored before.
var liveOnlyKinds = map[string]bool{
	KindApparentPower:  true,
	KindReactivePower:  true,
	KindPowerFactor:    true,
	KindDewPoint:       true,
	KindDcBusVoltage:   true,
	KindDcShuntVoltage: true,
	KindColorRed:       true,
	KindColorGreen:     true,
	KindColorBlue:      true,
	KindColorClear:     true,
}

// IsLiveOnly reports whether series of the kind keep no history (see liveOnlyKinds).
func IsLiveOnly(kind string) bool {
	return liveOnlyKinds[kind]
}

// LiveOnlyKinds lists the live-only kinds, for queries.
func LiveOnlyKinds() []string {
	kinds := make([]string, 0, len(liveOnlyKinds))
	for k := range liveOnlyKinds {
		kinds = append(kinds, k)
	}
	return kinds
}

// energyKinds are the ENERGY (AC meter) fields with a kind of their own.
var energyKinds = map[string]string{
	"Power":         KindPower,
	"ApparentPower": KindApparentPower,
	"ReactivePower": KindReactivePower,
	"Factor":        KindPowerFactor,
	"Voltage":       KindVoltage,
	"Current":       KindCurrent,
}

// dcKinds are the Voltage/Current/Power fields of any non-ENERGY sensor (INA219, INA226, ...).
var dcKinds = map[string]string{
	"Voltage": KindDcVoltage,
	"Current": KindDcCurrent,
	"Power":   KindDcPower,
}

// fieldKinds match by field name, any source (Temperature of ESP32 is chip_temperature instead).
var fieldKinds = map[string]string{
	"Temperature":      KindTemperature,
	"Humidity":         KindHumidity,
	"DewPoint":         KindDewPoint,
	"CarbonDioxide":    KindCo2,
	"Pressure":         KindPressure,
	"Illuminance":      KindIlluminance,
	"ColorTemperature": KindColorTemp,
	"BusVoltage":       KindDcBusVoltage,
	"ShuntVoltage":     KindDcShuntVoltage,
	"Red":              KindColorRed,
	"Green":            KindColorGreen,
	"Blue":             KindColorBlue,
	"Clear":            KindColorClear,
	"Leak":             KindLeak,
	"Contact":          KindContact,
	"Battery":          KindBattery,
	"BatteryVoltage":   KindBatteryVoltage,
}

// energySkip are the ENERGY fields that aren't samples: counters (kWh totals, Wh since the previous
// message; Today/Yesterday stay in DeviceState / sensor_history) and import/export totals of
// bidirectional meters. TotalStartTime is a string, never in Raw anyway; listed for completeness.
var energySkip = map[string]bool{
	"Total":          true,
	"Today":          true,
	"Yesterday":      true,
	"Period":         true,
	"TotalStartTime": true,
	"ImportActive":   true, // kWh counters of bidirectional meters (SDM, DDS238, ...)
	"ExportActive":   true,
	"ImportReactive": true,
	"ExportReactive": true,
	"TotalReactive":  true,
}

// anySkip are fields skipped for any source.
var anySkip = map[string]bool{
	"Id": true, // I2C address / sensor id (INA219 "Id":40, ...), not a measurement
}

// baseField splits a trailing number off a field name: arrays in a payload become Field1, Field2,
// ... (multi-phase ENERGY "Voltage":[230,231] -> Voltage1, Voltage2; ANALOG Temperature1, ...).
// It returns the field unchanged when there is no such suffix (or the name is all digits).
func baseField(field string) string {
	base := strings.TrimRight(field, "0123456789")
	if base == "" {
		return field
	}
	return base
}

// SeriesKind returns the kind of a series from its source (sensor object name) and field; unknown
// fields are KindRaw.
//
// Numbered fields (Voltage1, Temperature2, ...) are matched by their base name when the base is a
// known field: for ENERGY any of its fields (multi-phase meters, every phase its own series of the
// same kind), elsewhere only the unambiguous quantities of fieldKinds (ANALOG Temperature1,
// Illuminance1, ...). Numbered Voltage/Current/Power of a non-ENERGY sensor stay raw: such arrays
// come from AC inputs (ANALOG CT, PZEM, ...), not from a DC monitor.
func SeriesKind(source, field string) string {
	if source == SourceMq && field == "Ratio" {
		return KindGasRatio
	}
	if source == SourceEnergy {
		if kind, ok := energyKinds[field]; ok {
			return kind
		}
		if kind, ok := energyKinds[baseField(field)]; ok {
			return kind
		}
	} else if kind, ok := dcKinds[field]; ok {
		return kind
	}

	kind, ok := fieldKinds[field]
	if !ok {
		kind, ok = fieldKinds[baseField(field)]
	}
	if !ok {
		return KindRaw
	}
	if kind == KindTemperature && source == SourceChip {
		return KindChipTemperature
	}
	return kind
}

// SkipSeriesField reports whether a field isn't a measurement and gets no series. This is the
// one skip list: ENERGY counters (Total, Today, Yesterday, Period, TotalStartTime, import/export
// totals; numbered per phase too, Today1, ...) and the Id of any sensor.
func SkipSeriesField(source, field string) bool {
	if anySkip[field] {
		return true
	}
	if source == SourceEnergy {
		return energySkip[field] || energySkip[baseField(field)]
	}
	return false
}
