package mqtt

import (
	"fmt"
	"log/slog"
	"smart-home/config"
	"sync"

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

type MqttConsumer struct {
	Storage *Storage
	Client  mqtt.Client
	wg      *sync.WaitGroup
	topics  []string
}

func NewMqttConsumer(s *Storage, c mqtt.Client) *MqttConsumer {
	return &MqttConsumer{
		Storage: s,
		Client:  c,
		wg:      &sync.WaitGroup{},
		topics:  []string{},
	}

}

func (c *MqttConsumer) Run(cfg *config.Config) error {
	for _, device := range cfg.Devices {
		topic := fmt.Sprintf("tele/%s/SENSOR", device.Topic)
		token := c.Client.Subscribe(topic, 1, GetSensorsHandler(c.wg, &device, c.Storage))
		token.Wait()
		slog.Debug("Subscribed to topic", "topic", topic)
		c.topics = append(c.topics, topic)
	}

	slog.Info("Mqtt listener started")

	return nil
}

func (c *MqttConsumer) Shutdown() {
	for _, topic := range c.topics {
		c.Client.Unsubscribe(topic)
	}
	c.Client.Disconnect(250)
	slog.Info("Mqqt unsubscribed and disconnected")

	// Wait for the goroutine to finish
	c.wg.Wait()
	c.Storage.Shutdown()
	slog.Info("Storage flushed")
}
