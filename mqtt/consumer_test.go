package mqtt

import (
	"slices"
	"smart-home/model"
	"testing"
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
}
