package mqtt

import (
	"fmt"
	"log/slog"
	"smart-home/internal"

	driver "github.com/eclipse/paho.mqtt.golang"
)

type Consumer struct {
	topics    []string
	deviceMap *internal.DeviceStateManager
	parser    *EventParser
}

func NewMqttConsumer(
	deviceMap *internal.DeviceStateManager,
	parser *EventParser,
) *Consumer {
	return &Consumer{
		topics:    []string{},
		deviceMap: deviceMap,
		parser:    parser,
	}
}

func (c *Consumer) Subscribe(client driver.Client) {
	slog.Info("Connected to MQTT Broker")

	topicsToSubscribe := map[string]driver.MessageHandler{
		"tele/%s/SENSOR":   c.parser.parseSensorEvent,
		"stat/%s/POWER":    c.parser.parsePowerEvent,
		"stat/%s/RESULT":   c.parser.parseResult,
		"stat/%s/STATUS10": c.parser.parseState,
		"stat/%s/STATUS2":  c.parser.parseFirmware,
	}

	// ToDo: move out of here, remove states dependency
	for _, device := range c.deviceMap.GetAll() {
		for topicTpl, handler := range topicsToSubscribe {
			topic := fmt.Sprintf(topicTpl, device.Device.Topic)
			token := client.Subscribe(topic, 1, handler)
			token.Wait()
			slog.Debug("Subscribed to topic", "topic", topic)
			c.topics = append(c.topics, topic)
		}
	}

	slog.Info("Mqtt consumer subscribed!")
}

func (c *Consumer) Shutdown(client driver.Client) {
	for _, topic := range c.topics {
		client.Unsubscribe(topic)
	}
	client.Disconnect(250)
	slog.Info("Mqqt consumer unsubscribed")
}
