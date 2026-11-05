package mqtt

import (
	"fmt"
	"log/slog"
	"smart-home/internal"
	"smart-home/model"
	"sync"

	driver "github.com/eclipse/paho.mqtt.golang"
)

type Consumer struct {
	deviceMap *internal.DeviceStateManager
	handlers  map[string]driver.MessageHandler

	lock       sync.Mutex
	client     driver.Client   // set once connected
	subscribed map[uint]string // device id => device topic it's subscribed with
}

func NewMqttConsumer(
	deviceMap *internal.DeviceStateManager,
	parser *EventParser,
) *Consumer {
	return &Consumer{
		deviceMap:  deviceMap,
		subscribed: map[uint]string{},
		handlers: map[string]driver.MessageHandler{
			"tele/%s/SENSOR":   parser.parseSensorEvent,
			"tele/%s/LWT":      parser.parseLwt,
			"tele/%s/RESULT":   parser.parseTeleResult,
			"stat/%s/POWER":    parser.parsePowerEvent,
			"stat/%s/RESULT":   parser.parseResult,
			"stat/%s/STATUS2":  parser.parseFirmware,
			"stat/%s/STATUS10": parser.parseState,

			"stat/%s/STATUS":  parser.parseAsWarning,
			"stat/%s/STATUS0": parser.parseAsWarning,
			"stat/%s/STATUS1": parser.parseAsWarning,
			"stat/%s/STATUS3": parser.parseAsWarning,
			"stat/%s/STATUS4": parser.parseAsWarning,
			"stat/%s/STATUS7": parser.parseAsWarning,
			"stat/%s/TIMERS":  parser.parseTimers,
		},
	}
}

// Subscribe is the on-connect handler. The session is clean, so every connect
// and reconnect starts with no subscriptions on the broker side.
func (c *Consumer) Subscribe(client driver.Client) {
	slog.Info("Connected to MQTT Broker")

	c.lock.Lock()
	defer c.lock.Unlock()

	c.client = client
	c.subscribed = map[uint]string{}

	// ToDo: move out of here, remove states dependency
	for _, device := range c.deviceMap.GetAll() {
		if device.Device.Enabled {
			c.subscribe(device.Device)
		}
	}

	slog.Info("Mqtt consumer subscribed!")
}

// Sync brings the subscriptions of a created or updated device in line with its
// Enabled flag and topic. Returns true if the device has just been subscribed.
// Failures are only logged: the next reconnect subscribes from the current state.
func (c *Consumer) Sync(device *model.Device) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.client == nil {
		return false
	}

	topic, ok := c.subscribed[device.ID]
	if ok && (!device.Enabled || topic != device.Topic) {
		c.unsubscribe(device.ID, topic)
		ok = false
	}

	return device.Enabled && !ok && c.subscribe(device)
}

// Remove drops the subscriptions of a deleted device.
func (c *Consumer) Remove(deviceId uint) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if topic, ok := c.subscribed[deviceId]; ok {
		c.unsubscribe(deviceId, topic)
	}
}

func (c *Consumer) subscribe(device *model.Device) bool {
	for topicTpl, handler := range c.handlers {
		topic := fmt.Sprintf(topicTpl, device.Topic)
		token := c.client.Subscribe(topic, 1, handler)
		if token.Wait(); token.Error() != nil {
			slog.Error("Can't subscribe to topic", "topic", topic, "err", token.Error())
			return false
		}
		slog.Debug("Subscribed to topic", "topic", topic)
	}

	c.subscribed[device.ID] = device.Topic
	return true
}

func (c *Consumer) unsubscribe(deviceId uint, deviceTopic string) {
	topics := make([]string, 0, len(c.handlers))
	for topicTpl := range c.handlers {
		topics = append(topics, fmt.Sprintf(topicTpl, deviceTopic))
	}

	token := c.client.Unsubscribe(topics...)
	if token.Wait(); token.Error() != nil {
		slog.Error("Can't unsubscribe from device", "topic", deviceTopic, "err", token.Error())
	}
	delete(c.subscribed, deviceId)
}
