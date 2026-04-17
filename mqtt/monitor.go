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
					if state.Online == false { // Already offline
						continue
					}

					// should be updated already
					if state.LastUpdate == nil || now.Sub(*state.LastUpdate) > time.Minute {
						slog.Warn("Marking as MIA", "deviceId", state.Device.ID)
						state.Online = false
						state.On = nil
						state.Current = nil
						state.Voltage = nil
						state.Power = nil
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
