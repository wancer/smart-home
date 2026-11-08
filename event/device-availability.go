package event

// DeviceAvailability is dispatched when a device switches between online and offline.
type DeviceAvailability struct {
	DeviceId uint
	Online   bool
}
