package web

import "smart-home/model"

type SensorEvent struct {
	DeviceId  uint    `json:"DeviceId"`
	Timestamp int64   `json:"DeviceTime"`
	Period    uint    `json:"Period"`
	Power     uint    `json:"Power"`
	Current   float32 `json:"Current"`
	Voltage   uint    `json:"Voltage"`
}

func NewSensorEvent(dbRecord *model.SensorEventModel) *SensorEvent {
	return &SensorEvent{
		DeviceId:  dbRecord.DeviceId,
		Timestamp: dbRecord.DeviceTime.Unix(),
		Period:    dbRecord.Period,
		Power:     dbRecord.Power,
		Current:   dbRecord.Current,
		Voltage:   dbRecord.Voltage,
	}
}

type Device struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}
