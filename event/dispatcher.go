package event

import (
	"fmt"
	"log/slog"
	"reflect"
)

type Dispatcher struct {
	handlers map[string][]func(any)
}

type MqttEvent struct {
	Topic   string
	Payload any
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: map[string][]func(any){}}
}

func (d *Dispatcher) DispatchMqqt(event any, topic string) {
	eventName := getType(event)
	handlers, ok := d.handlers[eventName]
	if !ok {
		slog.Warn(fmt.Sprintf("no handlers for %s", eventName))
		return
	}

	e := MqttEvent{
		Topic:   topic,
		Payload: event,
	}

	for _, handler := range handlers {
		handler(e)
	}
}

func (d *Dispatcher) Dispatch(event any) {
	eventName := getType(event)
	handlers, ok := d.handlers[eventName]
	if !ok {
		slog.Warn(fmt.Sprintf("no handlers for %s", eventName))
		return
	}

	for _, handler := range handlers {
		handler(event)
	}
}

func (d *Dispatcher) Subscribe(eventName string, handler func(any)) {
	_, ok := d.handlers[eventName]
	if !ok {
		d.handlers[eventName] = []func(any){}
	}

	d.handlers[eventName] = append(d.handlers[eventName], handler)
}

func getType(myvar any) string {
	if t := reflect.TypeOf(myvar); t.Kind() == reflect.Pointer {
		return t.Elem().Name()
	} else {
		panic("Should be pointer, got " + t.Kind().String())
	}
}
