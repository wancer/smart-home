package handler

import (
	"smart-home/event"
	"smart-home/internal"
)

// IrReceivedHandler is implemented by automation.IrRuleRunner.
type IrReceivedHandler interface {
	Handle(*internal.DeviceState, *event.IrReceived)
}

func (h *EventHandler) HandleIrReceived(state *internal.DeviceState, e *event.IrReceived) {
	if h.ir != nil {
		h.ir.Handle(state, e)
	}
}
