package mqtt

import (
	"fmt"
	"log/slog"
	"slices"
	"smart-home/internal"
	"smart-home/model"
	"sync"

	driver "github.com/eclipse/paho.mqtt.golang"
)

type Consumer struct {
	deviceMap *internal.DeviceStateManager
	handlers  map[string]driver.MessageHandler

	lock       sync.Mutex
	client     driver.Client         // set once connected
	subscribed map[uint]subscription // device id => what it's subscribed with
}

// subscription is the topic and platform a device is subscribed with.
type subscription struct {
	topic    string
	platform string
}

// esphomeTopics are the topics an ESPHome device publishes (Tasmota-shaped, from its YAML).
var esphomeTopics = []string{"tele/%s/SENSOR", "tele/%s/LWT", "tele/%s/STATE", "tele/%s/INFO1", "tele/%s/INFO3"}

// esphomeOnlyTopics carry what Tasmota devices are polled for (STATUS2, STATUS1).
var esphomeOnlyTopics = []string{"tele/%s/INFO1", "tele/%s/INFO3"}

func NewMqttConsumer(
	deviceMap *internal.DeviceStateManager,
	parser *EventParser,
) *Consumer {
	return &Consumer{
		deviceMap:  deviceMap,
		subscribed: map[uint]subscription{},
		handlers: map[string]driver.MessageHandler{
			"tele/%s/SENSOR":   parser.parseSensorEvent,
			"tele/%s/LWT":      parser.parseLwt,
			"tele/%s/RESULT":   parser.parseTeleResult,
			"tele/%s/STATE":    parser.parseTeleState,
			"tele/%s/INFO1":    parser.parseInfo,
			"tele/%s/INFO3":    parser.parseInfo,
			"stat/%s/POWER":    parser.parsePowerEvent,
			"stat/%s/RESULT":   parser.parseResult,
			"stat/%s/STATUS1":  parser.parseStatus1,
			"stat/%s/STATUS2":  parser.parseFirmware,
			"stat/%s/STATUS10": parser.parseState,
			"stat/%s/STATUS11": parser.parseStatus11,

			"stat/%s/STATUS":  parser.parseAsWarning,
			"stat/%s/STATUS0": parser.parseAsWarning,
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
	c.subscribed = map[uint]subscription{}

	// ToDo: move out of here, remove states dependency
	for _, device := range c.deviceMap.GetAll() {
		if device.Device.Enabled {
			c.subscribe(device.Device)
		}
	}

	slog.Info("Mqtt consumer subscribed!")
}

// Sync brings the subscriptions of a created or updated device in line with its
// Enabled flag, topic and platform. Returns true if the device has just been subscribed.
// Failures are only logged: the next reconnect subscribes from the current state.
func (c *Consumer) Sync(device *model.Device) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.client == nil {
		return false
	}

	sub, ok := c.subscribed[device.ID]
	if ok && (!device.Enabled || sub != subscriptionOf(device)) {
		c.unsubscribe(device.ID, sub)
		ok = false
	}

	return device.Enabled && !ok && c.subscribe(device)
}

// Remove drops the subscriptions of a deleted device.
func (c *Consumer) Remove(deviceId uint) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if sub, ok := c.subscribed[deviceId]; ok {
		c.unsubscribe(deviceId, sub)
	}
}

func subscriptionOf(device *model.Device) subscription {
	return subscription{topic: device.Topic, platform: device.Platform}
}

// topicTemplates are the topic templates (keys of c.handlers) a platform subscribes to.
func (c *Consumer) topicTemplates(platform string) []string {
	if platform == model.PlatformEsphome {
		return esphomeTopics
	}
	templates := make([]string, 0, len(c.handlers))
	for topicTpl := range c.handlers {
		if !slices.Contains(esphomeOnlyTopics, topicTpl) {
			templates = append(templates, topicTpl)
		}
	}
	return templates
}

func (c *Consumer) subscribe(device *model.Device) bool {
	for _, topicTpl := range c.topicTemplates(device.Platform) {
		handler := c.handlers[topicTpl]
		topic := fmt.Sprintf(topicTpl, device.Topic)
		token := c.client.Subscribe(topic, 1, handler)
		if token.Wait(); token.Error() != nil {
			slog.Error("Can't subscribe to topic", "topic", topic, "err", token.Error())
			return false
		}
		slog.Debug("Subscribed to topic", "topic", topic)
	}

	c.subscribed[device.ID] = subscriptionOf(device)
	return true
}

func (c *Consumer) unsubscribe(deviceId uint, sub subscription) {
	templates := c.topicTemplates(sub.platform)
	topics := make([]string, 0, len(templates))
	for _, topicTpl := range templates {
		topics = append(topics, fmt.Sprintf(topicTpl, sub.topic))
	}

	token := c.client.Unsubscribe(topics...)
	if token.Wait(); token.Error() != nil {
		slog.Error("Can't unsubscribe from device", "topic", sub.topic, "err", token.Error())
	}
	delete(c.subscribed, deviceId)
}
