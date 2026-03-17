package model

type DeviceModel struct {
	ID    uint   `gorm:"primaryKey"`
	Name  string `gorm:"size:255;not null"`
	Topic string `gorm:"uniqueIndex;size:255;not null"`
}

func (DeviceModel) TableName() string {
	return "device"
}
