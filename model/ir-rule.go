package model

const (
	IrActionPower = "power" // switch a socket: Value on|off|toggle, TargetDeviceId
	IrActionIR    = "ir"    // send a stored IR signal: SignalId
	IrActionDelay = "delay" // wait DelayMs before the next action

	IrPowerOn     = "on"
	IrPowerOff    = "off"
	IrPowerToggle = "toggle"
)

// IrRule runs a list of actions when an IR device receives Code.
type IrRule struct {
	ID       uint   `gorm:"primaryKey"`
	DeviceId uint   `gorm:"not null;index"` // receiving IR device
	Name     string `gorm:"size:255;not null"`
	Code     string `gorm:"size:32;not null;index"` // 0x00F740BF
	Enabled  bool   `gorm:"not null;default:true"`
}

func (IrRule) TableName() string { return "ir_rule" }

// IrRuleAction is one ordered step of a rule.
type IrRuleAction struct {
	ID             uint   `gorm:"primaryKey"`
	RuleId         uint   `gorm:"not null;index"`
	Position       int    `gorm:"not null"`
	Type           string `gorm:"size:16;not null"`
	TargetDeviceId *uint
	Value          string `gorm:"size:16"`
	SignalId       *uint
	DelayMs        int
}

func (IrRuleAction) TableName() string { return "ir_rule_action" }
