package model

// IrSignal is a stored infra-red code that can be sent through an IR device.
// Signals are organised as Group -> Name, e.g. "Stereo" -> "Volume +".
type IrSignal struct {
	ID       uint   `gorm:"primaryKey"`
	DeviceId uint   `gorm:"not null;uniqueIndex:idx_ir_signal_name,priority:1;index"`
	Group    string `gorm:"column:group_name;size:255;not null;uniqueIndex:idx_ir_signal_name,priority:2"`
	Name     string `gorm:"size:255;not null;uniqueIndex:idx_ir_signal_name,priority:3"`
	Code     string `gorm:"size:32;not null"` // e.g. 0x00F740BF
	Protocol string `gorm:"size:32;not null;default:NEC"`
	Bits     int    `gorm:"not null;default:32"`
	Position int    `gorm:"not null;default:0"`
}

func (IrSignal) TableName() string {
	return "ir_signal"
}
