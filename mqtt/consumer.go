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

type MqttConsumer struct {
	Client    mqtt.Client
	topics    []string
	deviceMap *internal.DeviceMap
	handler   *EventHandler
}

func NewMqttConsumer(
	c mqtt.Client,
	deviceMap *internal.DeviceMap,
	handler *EventHandler,
) *MqttConsumer {
	return &MqttConsumer{
		Client:    c,
		topics:    []string{},
		deviceMap: deviceMap,
		handler:   handler,
	}
}

func (c *MqttConsumer) Run() error {
	for _, device := range c.deviceMap.GetAll() {
		topic := fmt.Sprintf("tele/%s/SENSOR", device.Topic)
		token := c.Client.Subscribe(topic, 1, c.handler.handleSensorEvent)
		token.Wait()
		slog.Debug("Subscribed to topic", "topic", topic)
		c.topics = append(c.topics, topic)
	}

	slog.Info("Mqtt listener started")

	for _, device := range c.deviceMap.GetAll() {
		topic := fmt.Sprintf("stat/%s/POWER", device.Topic)
		token := c.Client.Subscribe(topic, 1, c.handler.handlePowerEvent)
		token.Wait()
		slog.Debug("Subscribed to topic", "topic", topic)
		c.topics = append(c.topics, topic)
	}

	for _, device := range c.deviceMap.GetAll() {
		topic := fmt.Sprintf("cmnd/%s/POWER", device.Topic)
		token := c.Client.Publish(topic, 1, false, "")
		token.Wait()
		slog.Debug("Send to topic", "topic", topic)
		c.topics = append(c.topics, topic)
	}

	return nil
}

func (c *MqttConsumer) Shutdown() {
	for _, topic := range c.topics {
		c.Client.Unsubscribe(topic)
	}
	c.Client.Disconnect(250)
	slog.Info("Mqqt unsubscribed and disconnected")
}
