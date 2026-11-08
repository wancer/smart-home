package event

// SensorsUpdated is dispatched after a device's live sensor values were refreshed from telemetry.
type SensorsUpdated struct {
	DeviceId uint
}
