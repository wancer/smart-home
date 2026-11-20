package handler

import (
	"reflect"
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"strings"
)

// ToDo: find a better way
type EventPublisher interface {
	PublishStates(*model.Device)
}

type EventHandler struct {
	dispatcher *event.Dispatcher
	states     *internal.DeviceStateManager
	p          EventPublisher
	storage    *internal.Storage
	profiles   *internal.DeviceProfiles
	ir         IrReceivedHandler
}

func NewEventHandler(
	dispatcher *event.Dispatcher,
	states *internal.DeviceStateManager,
	p EventPublisher, // interface
	storage *internal.Storage,
	profiles *internal.DeviceProfiles,
	ir IrReceivedHandler,
) *EventHandler {
	return &EventHandler{
		dispatcher: dispatcher,
		states:     states,
		p:          p,
		storage:    storage,
		profiles:   profiles,
		ir:         ir,
	}
}

func toValue[T any](v reflect.Value) T {
	return v.Convert(reflect.TypeFor[T]()).Interface().(T)
}

func (h *EventHandler) Subscribe() {
	t := reflect.ValueOf(h)
	for m := range t.Methods() {
		if !strings.HasPrefix(m.Name, "Handle") {
			continue
		}

		h.dispatcher.Subscribe(
			m.Type.In(2).Elem().Name(),
			func(raw any) {
				e := raw.(event.MqttEvent)
				deviceState := h.states.GetByTopic(e.Topic)
				if deviceState == nil { // in-flight message of a deleted or renamed device
					return
				}

				t.MethodByName(m.Name).Call(
					[]reflect.Value{
						reflect.ValueOf(deviceState),
						reflect.ValueOf(e.Payload),
					},
				)
			},
		)
	}

	h.dispatcher.Subscribe(
		"InternalOfflineEvent",
		func(raw any) {
			e := raw.(*event.InternalOfflineEvent)
			h.InternalHandleOffline(e)
		},
	)
}
