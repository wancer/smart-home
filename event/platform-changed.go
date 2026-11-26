package event

// PlatformChanged: a device's platform was detected from its telemetry (Tasmota -> Matter), so its
// MQTT subscriptions must follow.
type PlatformChanged struct {
	DeviceId uint
}
