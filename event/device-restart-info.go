package event

// DeviceRestartInfo is dispatched when a device reports its uptime (tele/STATE, STATUS11)
// or its restart reason (STATUS1).
type DeviceRestartInfo struct {
	DeviceId uint
}
