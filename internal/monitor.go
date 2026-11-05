package internal

import (
	"log/slog"
	"smart-home/event"
	"smart-home/model"
	"time"
)

type WsBroadcaster interface {
	Send(channel string, in any)
}

type StateMonitor struct {
	dispatcher *event.Dispatcher
	states     *DeviceStateManager
}

func NewStateMonitor(dispatcher *event.Dispatcher, states *DeviceStateManager) *StateMonitor {
	return &StateMonitor{dispatcher: dispatcher, states: states}
}

func (m *StateMonitor) Run() chan struct{} {
	ticker := time.NewTicker(1 * time.Minute)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				now := time.Now()
				for _, state := range m.states.GetAll() {
					if state.Online == false { // Already offline
						continue
					}

					// availability comes from LWT, which stops arriving once disabled
					if state.Device.SensorType == model.SensorTypeIR && state.Device.Enabled {
						continue
					}

					// should be updated already
					if !state.Device.Enabled || state.LastUpdate == nil || now.Sub(*state.LastUpdate) > time.Minute {
						slog.Warn("Marking as MIA", "deviceId", state.Device.ID)
						event := &event.InternalOfflineEvent{
							DeviceId: state.Device.ID,
						}
						m.dispatcher.Dispatch(event)
						continue
					}
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

	return quit
}
