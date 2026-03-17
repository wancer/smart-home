package web

type SensorEvent struct {
	DeviceId  uint    `json:"DeviceId"`
	Timestamp int64   `json:"DeviceTime"`
	Period    uint    `json:"Period"`
	Power     uint    `json:"Power"`
	Current   float32 `json:"Current"`
}

type Device struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}
