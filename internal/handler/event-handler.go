package handler

import (
	"reflect"
	"smart-home/event"
	"smart-home/internal"
	"strings"
)

// ToDo: find a better way
type EventPublisher interface {
	PublishStates(*internal.Device)
}

type EventHandler struct {
	states  *internal.DeviceStateManager
	p       EventPublisher
	storage *internal.Storage
}

func NewEventHandler(
	states *internal.DeviceStateManager,
	p EventPublisher, // interface
	storage *internal.Storage,
) *EventHandler {
	return &EventHandler{
		states:  states,
		p:       p,
		storage: storage,
	}
}

func toValue[T any](v reflect.Value) T {
	return v.Convert(reflect.TypeFor[T]()).Interface().(T)
}

func (h *EventHandler) Subscribe(dispatcher *event.Dispatcher, states *internal.DeviceStateManager) {
	t := reflect.ValueOf(h)
	for m := range t.Methods() {
		if !strings.HasPrefix(m.Name, "Handle") {
			continue
		}

		dispatcher.Subscribe(
			m.Type.In(2).Elem().Name(),
			func(raw any) {
				e := raw.(event.MqttEvent)
				deviceState := states.GetByTopic(e.Topic)
				t.MethodByName(m.Name).Call(
					[]reflect.Value{
						reflect.ValueOf(deviceState),
						reflect.ValueOf(e.Payload),
					},
				)
			},
		)
	}

	dispatcher.Subscribe(
		"InternalOfflineEvent",
		func(raw any) {
			e := raw.(*event.InternalOfflineEvent)
			h.InternalHandleOffline(e)
		},
	)
}
