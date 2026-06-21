package mqtt

import (
	"fmt"
	"log/slog"
	"smart-home/model"

	driver "github.com/eclipse/paho.mqtt.golang"
)

type Publisher struct {
	client driver.Client // interface
}

func NewPublisher(client driver.Client) *Publisher {
	return &Publisher{client: client}
}

func (p *Publisher) PublishStates(device *model.Device) {
	p.GetOnOff(device)
	p.GetSensors(device)
}

func (p *Publisher) GetOnOff(device *model.Device) {
	p.publish(device.Topic, "POWER", "")
}

func (p *Publisher) OnOff(device *model.Device, state bool) {
	var value string
	if state {
		value = "ON"
	} else {
		value = "OFF"
	}

	p.publish(device.Topic, "POWER", value)
}

func (p *Publisher) GetSensors(device *model.Device) {
	p.publish(device.Topic, "STATUS10", "10")
}

func (p *Publisher) GetFirmware(device *model.Device) {
	p.publish(device.Topic, "STATUS2", "2")
}

func (p *Publisher) GetParameters(device *model.Device) {
	p.publish(device.Topic, "STATUS1", "1")
}

func (p *Publisher) GetTelemetry(device *model.Device) {
	p.publish(device.Topic, "STATUS3", "3")
}

func (p *Publisher) GetMemory(device *model.Device) {
	p.publish(device.Topic, "STATUS4", "4")
}

func (p *Publisher) GetTime(device *model.Device) {
	p.publish(device.Topic, "STATUS7", "")
}

func (p *Publisher) GetTimers(device *model.Device) {
	for i := 1; i <= 16; i++ {
		p.GetTimer(device, i)
	}
}

func (p *Publisher) GetRules(device *model.Device) {
	for i := 1; i <= 3; i++ {
		p.GetRule(device, i)
	}
}

func (p *Publisher) GetRule(device *model.Device, n int) {
	p.publish(device.Topic, fmt.Sprintf("Rule%d", n), "")
}

func (p *Publisher) SetRule(device *model.Device, n int, value string) {
	p.publish(device.Topic, fmt.Sprintf("Rule%d", n), value)
}

func (p *Publisher) GetTimer(device *model.Device, n int) {
	p.publish(device.Topic, fmt.Sprintf("Timer%d", n), "")
}

func (p *Publisher) SetTimer(device *model.Device, n int, value string) {
	p.publish(device.Topic, fmt.Sprintf("Timer%d", n), value)
}

func (p *Publisher) SetVoltage(device *model.Device, voltage int) {
	value := fmt.Sprintf("%d", voltage)
	p.publish(device.Topic, "VoltageSet", value)
}

func (p *Publisher) SetPower(device *model.Device, volts uint, power int) {
	value := fmt.Sprintf("%d, %d", power, volts)
	p.publish(device.Topic, "PowerSet", value)
}

func (p *Publisher) GetTimezone(device *model.Device) {
	p.publish(device.Topic, "Timezone", "")
}

func (p *Publisher) SetTimezone(device *model.Device, offset string) {
	p.publish(device.Topic, "Timezone", offset)
}

func (p *Publisher) GetTimeStd(device *model.Device) {
	p.publish(device.Topic, "TimeStd", "")
}

func (p *Publisher) SetTimeStd(device *model.Device, value string) {
	p.publish(device.Topic, "TimeStd", value)
}

func (p *Publisher) GetTimeDst(device *model.Device) {
	p.publish(device.Topic, "TimeDst", "")
}

func (p *Publisher) SetTimeDst(device *model.Device, value string) {
	p.publish(device.Topic, "TimeDst", value)
}

func (p *Publisher) GetLedPower(device *model.Device) {
	p.publish(device.Topic, "LedPower", "")
}

func (p *Publisher) GetLedState(device *model.Device) {
	p.publish(device.Topic, "LedState", "")
}

func (p *Publisher) GetTelePeriod(device *model.Device) {
	p.publish(device.Topic, "TelePeriod", "")
}

func (p *Publisher) SetTelePeriod(device *model.Device, value int) {
	formatted := fmt.Sprintf("%d", value)
	p.publish(device.Topic, "TelePeriod", formatted)
}

func (p *Publisher) GetLedPwmMode(device *model.Device) {
	p.publish(device.Topic, "LedPwmMode", "")
}

func (p *Publisher) SetLedPwmMode(device *model.Device, state bool) {
	var value string
	if state {
		value = "ON"
	} else {
		value = "OFF"
	}
	p.publish(device.Topic, "LedPwmMode", value)
}

func (p *Publisher) GetLedPwmOff(device *model.Device) {
	p.publish(device.Topic, "LedPwmOff", "")
}

func (p *Publisher) SetLedPwmOff(device *model.Device, value int) {
	formatted := fmt.Sprintf("%d", value)
	p.publish(device.Topic, "LedPwmOff", formatted)
}

func (p *Publisher) GetLedPwmOn(device *model.Device) {
	p.publish(device.Topic, "LedPwmOn", "")
}

func (p *Publisher) SetLedPwmOn(device *model.Device, value int) {
	formatted := fmt.Sprintf("%d", value)
	p.publish(device.Topic, "LedPwmOn", formatted)
}

func (p *Publisher) publish(device string, command string, value string) {
	topic := fmt.Sprintf("cmnd/%s/%s", device, command)
	token := p.client.Publish(topic, 1, false, value)
	token.Wait()

	slog.Debug("Send to topic", "topic", topic, "value", value)
}
