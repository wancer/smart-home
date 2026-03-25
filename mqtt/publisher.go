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
		p.GetTimezone(state.Device)
		p.GetTimeStd(state.Device)
		p.GetTimeDst(state.Device)
		p.GetLedPower(state.Device)
		p.GetLedState(state.Device)
		p.GetTelePeriod(state.Device)
		p.GetLedPwmMode(state.Device)
		p.GetLedPwmOff(state.Device)
		p.GetLedPwmOn(state.Device)
	}
}

func (p *Publisher) PublishStates(device *internal.Device) {
	p.GetOnOff(device)
	p.GetSensors(device)
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

func (p *Publisher) GetTimeStd(device *internal.Device) {
	p.publish(device.Topic, "TimeStd", "")
}

func (p *Publisher) GetTimeDst(device *internal.Device) {
	p.publish(device.Topic, "TimeDst", "")
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

func (p *Publisher) GetLedPwmMode(device *internal.Device) {
	p.publish(device.Topic, "LedPwmMode", "")
}

func (p *Publisher) GetLedPwmOff(device *internal.Device) {
	p.publish(device.Topic, "LedPwmOff", "")
}

func (p *Publisher) GetLedPwmOn(device *internal.Device) {
	p.publish(device.Topic, "LedPwmOn", "")
}

func (p *Publisher) publish(device string, command string, value string) {
	topic := fmt.Sprintf("cmnd/%s/%s", device, command)
	token := p.client.Publish(topic, 1, false, value)
	token.Wait()

	slog.Debug("Send to topic", "topic", topic, "value", value)
}
