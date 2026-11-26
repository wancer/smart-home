package mqtt

import (
	"slices"
	"smart-home/internal"
	"smart-home/model"
	"testing"

	driver "github.com/eclipse/paho.mqtt.golang"
)

func TestTopicTemplates(t *testing.T) {
	c := NewMqttConsumer(nil, &EventParser{})

	tasmota := c.topicTemplates(model.PlatformTasmota)
	if want := len(c.handlers) - len(esphomeOnlyTopics); len(tasmota) != want {
		t.Errorf("tasmota subscribes to %d topics, want %d", len(tasmota), want)
	}
	for _, tpl := range esphomeOnlyTopics {
		if slices.Contains(tasmota, tpl) {
			t.Errorf("tasmota subscribes to %s, which it's polled for instead", tpl)
		}
	}

	esphome := c.topicTemplates(model.PlatformEsphome)
	if len(esphome) != 5 {
		t.Errorf("esphome topics = %v, want SENSOR, LWT, STATE, INFO1, INFO3", esphome)
	}
	for _, tpl := range esphome {
		if c.handlers[tpl] == nil {
			t.Errorf("esphome topic %s has no handler", tpl)
		}
	}

	matter := c.topicTemplates(model.PlatformMatter)
	if want := []string{"stat/%s/RESULT", "tele/%s/INFO", "tele/%s/SENSOR"}; !slices.Equal(matter, want) {
		t.Errorf("matter topics = %v, want %v", matter, want)
	}
	for tpl := range c.bridgeHandlers {
		if tpl != "tele/%s/LWT" && tpl != "tele/%s/STATE" {
			t.Errorf("unexpected bridge topic %s", tpl)
		}
	}
}

type fakeMessage struct {
	driver.Message
	topic string
}

func (m fakeMessage) Topic() string   { return m.topic }
func (m fakeMessage) Payload() []byte { return []byte("Offline") }

// A bridge's LWT goes to every enabled Matter device behind it, as tele/<device>/LWT.
func TestBridgeFanOut(t *testing.T) {
	states := internal.NewDeviceStateManager([]*model.Device{
		{ID: 1, Topic: "klippbok", Platform: model.PlatformMatter, Enabled: true},
		{ID: 2, Topic: "door", Platform: model.PlatformMatter, Bridge: "matter-bridge", Enabled: true},
		{ID: 3, Topic: "other", Platform: model.PlatformMatter, Bridge: "bridge2", Enabled: true},
		{ID: 4, Topic: "off", Platform: model.PlatformMatter, Enabled: false},
		{ID: 5, Topic: "plug", Platform: model.PlatformTasmota, Enabled: true},
	})
	c := NewMqttConsumer(states, &EventParser{})
	var got []string
	handler := c.fanOut("LWT", func(_ driver.Client, msg driver.Message) { got = append(got, msg.Topic()) })
	handler(nil, fakeMessage{topic: "tele/matter-bridge/LWT"})
	slices.Sort(got)
	if want := []string{"tele/door/LWT", "tele/klippbok/LWT"}; !slices.Equal(got, want) {
		t.Errorf("fanned out to %v, want %v", got, want)
	}
}
