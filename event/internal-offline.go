package event

// Special event for the case when device went offline
type InternalOfflineEvent struct {
	DeviceId uint
}
