package event

// SeriesChanged is dispatched when a device got a new sensor series or a series' label/hidden
// flag changed, so clients get the new series metadata.
type SeriesChanged struct {
	DeviceId uint
}
