package internal

import (
	"log/slog"
	"smart-home/event"
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
	ticker := time.NewTicker(15 * time.Second)
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

					// IR / unrecognised device availability comes from LWT, which stops arriving once
					// disabled. If the device also sends telemetry (chip temperature), missing
					// telemetry counts too.
					if state.Device.AvailabilityFromLwt() && state.Device.Enabled {
						if state.LastTelemetry == nil || now.Sub(*state.LastTelemetry) <= OfflineTimeout(state) {
							continue
						}
						slog.Warn("Marking as MIA: no telemetry", "deviceId", state.Device.ID)
						m.dispatcher.Dispatch(&event.InternalOfflineEvent{DeviceId: state.Device.ID})
						continue
					}

					// should be updated already
					if !state.Device.Enabled || state.LastUpdate == nil || now.Sub(*state.LastUpdate) > OfflineTimeout(state) {
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

// defaultTelePeriod is Tasmota's default TelePeriod, used until the device reports its own.
const defaultTelePeriod = 300 * time.Second

// OfflineTimeout is how long a device may stay silent before it's marked offline: the
// device's own setting, or by default two missed TelePeriods (at least a minute), so a
// longer TelePeriod doesn't make the device flap. Until the device reports its TelePeriod,
// Tasmota's default is assumed.
func OfflineTimeout(state *DeviceState) time.Duration {
	if state.Device.OfflineTimeout > 0 {
		return time.Duration(state.Device.OfflineTimeout) * time.Second
	}
	period := defaultTelePeriod
	if state.Config.TelePeriod != nil && *state.Config.TelePeriod > 0 {
		period = time.Duration(*state.Config.TelePeriod) * time.Second
	}
	return max(time.Minute, 2*period+30*time.Second)
}
