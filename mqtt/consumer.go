package mqtt

import (
	"fmt"
	"log/slog"
	"slices"
	"smart-home/internal"
	"smart-home/model"
	"strings"
	"sync"

	driver "github.com/eclipse/paho.mqtt.golang"
)

type Consumer struct {
	deviceMap *internal.DeviceStateManager
	handlers  map[string]driver.MessageHandler
	// matterHandlers are the per-sensor topics of a Matter device, bridgeHandlers the topics of
	// its bridge (fanned out to every sensor of the bridge).
	matterHandlers map[string]driver.MessageHandler
	bridgeHandlers map[string]driver.MessageHandler

	lock       sync.Mutex
	client     driver.Client         // set once connected
	subscribed map[uint]subscription // device id => what it's subscribed with
	bridges    map[string]int        // bridge topic => number of subscribed Matter devices behind it
}

// subscription is the topic and platform a device is subscribed with (and its bridge, Matter only).
type subscription struct {
	topic    string
	platform string
	bridge   string
}

// esphomeTopics are the topics an ESPHome device publishes (Tasmota-shaped, from its YAML).
var esphomeTopics = []string{"tele/%s/SENSOR", "tele/%s/LWT", "tele/%s/STATE", "tele/%s/INFO1", "tele/%s/INFO3"}

// esphomeOnlyTopics carry what Tasmota devices are polled for (STATUS2, STATUS1).
var esphomeOnlyTopics = []string{"tele/%s/INFO1", "tele/%s/INFO3"}

func NewMqttConsumer(
	deviceMap *internal.DeviceStateManager,
	parser *EventParser,
) *Consumer {
	c := &Consumer{
		deviceMap:  deviceMap,
		subscribed: map[uint]subscription{},
		bridges:    map[string]int{},
		matterHandlers: map[string]driver.MessageHandler{
			"tele/%s/SENSOR": parser.parseSensorEvent,
			"tele/%s/INFO":   parser.parseMatterInfo,
			"stat/%s/RESULT": parser.parseCommandResult,
		},
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
	c.bridgeHandlers = map[string]driver.MessageHandler{
		"tele/%s/LWT":   c.fanOut("LWT", parser.parseLwt),
		"tele/%s/STATE": c.fanOut("STATE", parser.parseTeleState),
	}
	return c
}

// deviceMessage is a bridge message re-addressed to one of its devices.
type deviceMessage struct {
	driver.Message
	topic string
}

func (m deviceMessage) Topic() string { return m.topic }

// fanOut hands a message of a bridge (tele/<bridge>/<suffix>) to handler once per enabled Matter
// device behind it, as if it came on tele/<device topic>/<suffix>.
func (c *Consumer) fanOut(suffix string, handler driver.MessageHandler) driver.MessageHandler {
	return func(client driver.Client, msg driver.Message) {
		bridge := topicName(msg.Topic())
		for _, state := range c.deviceMap.GetAll() {
			d := state.Device
			if d.Enabled && d.IsBridged() && d.BridgeTopic() == bridge {
				handler(client, deviceMessage{Message: msg, topic: "tele/" + d.Topic + "/" + suffix})
			}
		}
	}
}

// topicName is the middle part of prefix/<name>/suffix.
func topicName(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) < 3 {
		return ""
	}
	return strings.Join(parts[1:len(parts)-1], "/")
}

// Subscribe is the on-connect handler. The session is clean, so every connect
// and reconnect starts with no subscriptions on the broker side.
func (c *Consumer) Subscribe(client driver.Client) {
	slog.Info("Connected to MQTT Broker")

	c.lock.Lock()
	defer c.lock.Unlock()

	c.client = client
	c.subscribed = map[uint]subscription{}
	c.bridges = map[string]int{}

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
	sub := subscription{topic: device.Topic, platform: device.Platform}
	if device.IsBridged() {
		sub.bridge = device.BridgeTopic()
	}
	return sub
}

// topicHandlers are the topic templates a platform subscribes to (per device), with their handlers.
func (c *Consumer) topicHandlers(platform string) map[string]driver.MessageHandler {
	if platform == model.PlatformMatter {
		return c.matterHandlers
	}
	handlers := map[string]driver.MessageHandler{}
	for topicTpl, handler := range c.handlers {
		if platform == model.PlatformEsphome && !slices.Contains(esphomeTopics, topicTpl) ||
			platform != model.PlatformEsphome && slices.Contains(esphomeOnlyTopics, topicTpl) {
			continue
		}
		handlers[topicTpl] = handler
	}
	return handlers
}

// topicTemplates are the topic templates a platform subscribes to (per device), sorted.
func (c *Consumer) topicTemplates(platform string) []string {
	templates := make([]string, 0, len(c.handlers))
	for topicTpl := range c.topicHandlers(platform) {
		templates = append(templates, topicTpl)
	}
	slices.Sort(templates)
	return templates
}

func (c *Consumer) subscribe(device *model.Device) bool {
	sub := subscriptionOf(device)
	for topicTpl, handler := range c.topicHandlers(device.Platform) {
		if !c.subscribeTopic(fmt.Sprintf(topicTpl, device.Topic), handler) {
			return false
		}
	}
	if sub.bridge != "" {
		// subscribed again for every device: the broker resends the retained LWT, so a newly added
		// device learns whether its bridge is up
		for topicTpl, handler := range c.bridgeHandlers {
			if !c.subscribeTopic(fmt.Sprintf(topicTpl, sub.bridge), handler) {
				return false
			}
		}
		c.bridges[sub.bridge]++
	}

	c.subscribed[device.ID] = sub
	return true
}

func (c *Consumer) subscribeTopic(topic string, handler driver.MessageHandler) bool {
	token := c.client.Subscribe(topic, 1, handler)
	if token.Wait(); token.Error() != nil {
		slog.Error("Can't subscribe to topic", "topic", topic, "err", token.Error())
		return false
	}
	slog.Debug("Subscribed to topic", "topic", topic)
	return true
}

func (c *Consumer) unsubscribe(deviceId uint, sub subscription) {
	templates := c.topicTemplates(sub.platform)
	topics := make([]string, 0, len(templates))
	for _, topicTpl := range templates {
		topics = append(topics, fmt.Sprintf(topicTpl, sub.topic))
	}
	if sub.bridge != "" {
		c.bridges[sub.bridge]--
		if c.bridges[sub.bridge] <= 0 { // the last device of the bridge
			delete(c.bridges, sub.bridge)
			for topicTpl := range c.bridgeHandlers {
				topics = append(topics, fmt.Sprintf(topicTpl, sub.bridge))
			}
		}
	}

	token := c.client.Unsubscribe(topics...)
	if token.Wait(); token.Error() != nil {
		slog.Error("Can't unsubscribe from device", "topic", sub.topic, "err", token.Error())
	}
	delete(c.subscribed, deviceId)
}
