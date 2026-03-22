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
	deviceMap *internal.DeviceMap
	handler   *EventHandler
}

func NewMqttConsumer(
	client mqtt.Client,
	deviceMap *internal.DeviceMap,
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
	for _, device := range c.deviceMap.GetAll() {
		topic := fmt.Sprintf("tele/%s/SENSOR", device.Topic)
		token := c.client.Subscribe(topic, 1, c.handler.handleSensorEvent)
		token.Wait()
		slog.Debug("Subscribed to topic", "topic", topic)
		c.topics = append(c.topics, topic)
	}

	slog.Info("Mqtt listener started")

	for _, device := range c.deviceMap.GetAll() {
		topic := fmt.Sprintf("stat/%s/POWER", device.Topic)
		token := c.client.Subscribe(topic, 1, c.handler.handlePowerEvent)
		token.Wait()
		slog.Debug("Subscribed to topic", "topic", topic)
		c.topics = append(c.topics, topic)
	}

	for _, device := range c.deviceMap.GetAll() {
		topic := fmt.Sprintf("stat/%s/RESULT", device.Topic)
		token := c.client.Subscribe(topic, 1, c.handler.handleResult)
		token.Wait()
		slog.Debug("Subscribed to topic", "topic", topic)
		c.topics = append(c.topics, topic)
	}

	for _, device := range c.deviceMap.GetAll() {
		topic := fmt.Sprintf("stat/%s/STATUS10", device.Topic)
		token := c.client.Subscribe(topic, 1, c.handler.handleState)
		token.Wait()
		slog.Debug("Subscribed to topic", "topic", topic)
		c.topics = append(c.topics, topic)
	}

	return nil
}

func (c *Consumer) Shutdown() {
	for _, topic := range c.topics {
		c.client.Unsubscribe(topic)
	}
	c.client.Disconnect(250)
	slog.Info("Mqqt unsubscribed and disconnected")
}
