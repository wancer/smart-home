package mqtt

import (
	"fmt"
	"log/slog"
	"smart-home/internal"
	"smart-home/model"

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
		p.SendGetPower(device)
	}
}

func (p *Publisher) SendGetPower(device *model.DeviceModel) {
	topic := fmt.Sprintf("cmnd/%s/POWER", device.Topic)
	token := p.client.Publish(topic, 1, false, "")
	token.Wait()
	slog.Debug("Send to topic", "topic", topic)
}

func (p *Publisher) SendSetPower(device *model.DeviceModel, state bool) {
	var value string
	if state {
		value = "ON"
	} else {
		value = "OFF"
	}

	topic := fmt.Sprintf("cmnd/%s/POWER", device.Topic)
	token := p.client.Publish(topic, 1, false, value)
	token.Wait()
	slog.Debug("Send to topic", "topic", topic)
}
