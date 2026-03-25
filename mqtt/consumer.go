package mqtt

import (
	"fmt"
	"log/slog"
	"smart-home/internal"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var mqttMsgChan = make(chan mqtt.Message)

var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	mqttMsgChan <- msg
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	slog.Debug("Connected to MQTT Broker")
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	slog.Info("Connection lost", "err", err)
}

type WsBroadcaster interface {
	Send(channel string, in any)
}

type Consumer struct {
	client    mqtt.Client // interface
	topics    []string
	deviceMap *internal.DeviceStateStorage
	handler   *EventHandler
}

func NewMqttConsumer(
	client mqtt.Client,
	deviceMap *internal.DeviceStateStorage,
	handler *EventHandler,
) *Consumer {
	return &Consumer{
		client:    client,
		topics:    []string{},
		deviceMap: deviceMap,
		handler:   handler,
	}
}

func (c *Consumer) Run() error {
	topicsToSubscribe := map[string]mqtt.MessageHandler{
		"tele/%s/SENSOR":   c.handler.handleSensorEvent,
		"stat/%s/POWER":    c.handler.handlePowerEvent,
		"stat/%s/RESULT":   c.handler.handleResult,
		"stat/%s/STATUS10": c.handler.handleState,
	}

	for _, device := range c.deviceMap.GetAll() {
		for topicTpl, handler := range topicsToSubscribe {
			topic := fmt.Sprintf(topicTpl, device.Device.Topic)
			token := c.client.Subscribe(topic, 1, handler)
			token.Wait()
			slog.Debug("Subscribed to topic", "topic", topic)
			c.topics = append(c.topics, topic)
		}
	}

	slog.Info("Mqtt listener started")

	return nil
}

func (c *Consumer) Shutdown() {
	for _, topic := range c.topics {
		c.client.Unsubscribe(topic)
	}
	c.client.Disconnect(250)
	slog.Info("Mqqt consumer unsubscribed")
}
