package mqtt

import (
	"fmt"
	"log/slog"
	"smart-home/internal"
	"smart-home/model"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type Publisher struct {
	client    mqtt.Client // interface
	deviceMap *internal.DeviceMap
}

func NewPublisher(
	c mqtt.Client,
	deviceMap *internal.DeviceMap,
) *Publisher {
	return &Publisher{
		client:    c,
		deviceMap: deviceMap,
	}
}

func (p *Publisher) PublishAllStates() {
	for _, device := range p.deviceMap.GetAll() {
		p.GetOnOff(device)
		p.GetState(device)
	}
}

func (p *Publisher) GetOnOff(device *model.DeviceModel) {
	p.publish(device.Topic, "POWER", "")
}

func (p *Publisher) OnOff(device *model.DeviceModel, state bool) {
	var value string
	if state {
		value = "ON"
	} else {
		value = "OFF"
	}

	p.publish(device.Topic, "POWER", value)
}

func (p *Publisher) GetState(device *model.DeviceModel) {
	p.publish(device.Topic, "STATUS10", "10")
}

func (p *Publisher) SetVoltage(device *model.DeviceModel, voltage int) {
	value := fmt.Sprintf("%d", voltage)
	p.publish(device.Topic, "VoltageSet", value)

	time.Sleep(5 * time.Second)
	p.GetState(device)
}

func (p *Publisher) SetPower(device *model.DeviceModel, volts uint, power int) {
	value := fmt.Sprintf("%d, %d", volts, power)
	p.publish(device.Topic, "PowerSet", value)

	time.Sleep(5 * time.Second)
	p.GetState(device)
}

func (p *Publisher) publish(device string, command string, value string) {
	topic := fmt.Sprintf("cmnd/%s/%s", device, command)
	token := p.client.Publish(topic, 1, false, value)
	token.Wait()

	slog.Debug("Send to topic", "topic", topic, "value", value)
}
