package mqtt

import (
	"log/slog"
	"smart-home/internal"
	"time"
)

type StateMonitor struct {
	ws     WsBroadcaster // interface
	states *internal.DeviceStateStorage
}

func NewStateMonitor(ws WsBroadcaster, states *internal.DeviceStateStorage) *StateMonitor {
	return &StateMonitor{ws: ws, states: states}
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
					if state.On == nil {
						continue // already state unclear
					}
					if state.LastUpdate == nil {
						slog.Warn("Marging as MIA", "deviceId", state.Device.ID)
						state.On = nil // should be updated already
						m.ws.Send("state", state)
						continue
					}

					if now.Sub(*state.LastUpdate) > time.Minute {
						slog.Warn("Marging as MIA", "deviceId", state.Device.ID)
						state.On = nil // should be updated already
						m.ws.Send("state", state)
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
