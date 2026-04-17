package mqtt

import (
	"fmt"
	"log/slog"
	"smart-home/internal"

	driver "github.com/eclipse/paho.mqtt.golang"
)

type Publisher struct {
	client driver.Client // interface
	states *internal.DeviceStateStorage
}

func NewPublisher(
	c driver.Client,
	states *internal.DeviceStateStorage,
) *Publisher {
	return &Publisher{
		client: c,
		states: states,
	}
}

func (p *Publisher) PublishAllStates() {
	for _, state := range p.states.GetAll() {
		p.PublishStates(state.Device)
	}
}

func (p *Publisher) PublishStates(device *internal.Device) {
	p.GetOnOff(device)
	p.GetSensors(device)
	p.GetFirmware(device)
	p.GetTimezone(device)
	p.GetTimeStd(device)
	p.GetTimeDst(device)
	p.GetLedPower(device)
	p.GetLedState(device)
	p.GetTelePeriod(device)
	p.GetLedPwmMode(device)
	p.GetLedPwmOff(device)
	p.GetLedPwmOn(device)
}

func (p *Publisher) GetOnOff(device *internal.Device) {
	p.publish(device.Topic, "POWER", "")
}

func (p *Publisher) OnOff(device *internal.Device, state bool) {
	var value string
	if state {
		value = "ON"
	} else {
		value = "OFF"
	}

	p.publish(device.Topic, "POWER", value)
}

func (p *Publisher) GetSensors(device *internal.Device) {
	p.publish(device.Topic, "STATUS10", "10")
}

func (p *Publisher) GetFirmware(device *internal.Device) {
	p.publish(device.Topic, "STATUS2", "2")
}

func (p *Publisher) SetVoltage(device *internal.Device, voltage int) {
	value := fmt.Sprintf("%d", voltage)
	p.publish(device.Topic, "VoltageSet", value)
}

func (p *Publisher) SetPower(device *internal.Device, volts uint, power int) {
	value := fmt.Sprintf("%d, %d", power, volts)
	p.publish(device.Topic, "PowerSet", value)
}

func (p *Publisher) GetTimezone(device *internal.Device) {
	p.publish(device.Topic, "Timezone", "")
}

func (p *Publisher) SetTimezone(device *internal.Device, offset string) {
	p.publish(device.Topic, "Timezone", offset)
}

func (p *Publisher) GetTimeStd(device *internal.Device) {
	p.publish(device.Topic, "TimeStd", "")
}

func (p *Publisher) SetTimeStd(device *internal.Device, value string) {
	p.publish(device.Topic, "TimeStd", value)
}

func (p *Publisher) GetTimeDst(device *internal.Device) {
	p.publish(device.Topic, "TimeDst", "")
}

func (p *Publisher) SetTimeDst(device *internal.Device, value string) {
	p.publish(device.Topic, "TimeDst", value)
}

func (p *Publisher) GetLedPower(device *internal.Device) {
	p.publish(device.Topic, "LedPower", "")
}

func (p *Publisher) GetLedState(device *internal.Device) {
	p.publish(device.Topic, "LedState", "")
}

func (p *Publisher) GetTelePeriod(device *internal.Device) {
	p.publish(device.Topic, "TelePeriod", "")
}

func (p *Publisher) SetTelePeriod(device *internal.Device, value int) {
	formatted := fmt.Sprintf("%d", value)
	p.publish(device.Topic, "TelePeriod", formatted)
}

func (p *Publisher) GetLedPwmMode(device *internal.Device) {
	p.publish(device.Topic, "LedPwmMode", "")
}

func (p *Publisher) SetLedPwmMode(device *internal.Device, state bool) {
	var value string
	if state {
		value = "ON"
	} else {
		value = "OFF"
	}
	p.publish(device.Topic, "LedPwmMode", value)
}

func (p *Publisher) GetLedPwmOff(device *internal.Device) {
	p.publish(device.Topic, "LedPwmOff", "")
}

func (p *Publisher) SetLedPwmOff(device *internal.Device, value int) {
	formatted := fmt.Sprintf("%d", value)
	p.publish(device.Topic, "LedPwmOff", formatted)
}

func (p *Publisher) GetLedPwmOn(device *internal.Device) {
	p.publish(device.Topic, "LedPwmOn", "")
}

func (p *Publisher) SetLedPwmOn(device *internal.Device, value int) {
	formatted := fmt.Sprintf("%d", value)
	p.publish(device.Topic, "LedPwmOn", formatted)
}

func (p *Publisher) publish(device string, command string, value string) {
	topic := fmt.Sprintf("cmnd/%s/%s", device, command)
	token := p.client.Publish(topic, 1, false, value)
	token.Wait()

	slog.Debug("Send to topic", "topic", topic, "value", value)
}
