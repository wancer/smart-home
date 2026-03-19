package mqtt

import (
	"encoding/json"
	"log/slog"
	"smart-home/event"
	"smart-home/internal"
	"smart-home/model"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func GetSensorsHandler(
	wg *sync.WaitGroup,
	device *model.DeviceModel,
	storage *internal.Storage,
	ws WsBroadcaster, // interface
) func(client mqtt.Client, msg mqtt.Message) {
	var myHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
		wg.Add(1)
		defer wg.Done()

		var event *event.SensorEvent
		err := json.Unmarshal(msg.Payload(), &event)
		if err != nil {
			slog.Error(
				"CANT_PARSE_MSG",
				"err", err,
				"topic", msg.Topic(),
				"payload", msg.Payload(),
			)
			return
		}

		slog.Debug("sensor", "event", string(msg.Payload()))
		m := toModel(event, device)

		storage.Store(m)
		storage.StoreDaily(event, device)

		ws.Send("sensor", m)
	}

	return myHandler
}

func toModel(e *event.SensorEvent, d *model.DeviceModel) *model.SensorEventModel {
	now := time.Now()

	r := &model.SensorEventModel{}
	r.DeviceId = d.ID
	r.RealTime = now
	r.DeviceTime = time.Time(e.Time)
	r.Period = uint(e.Energy.Period)
	r.Power = uint(e.Energy.Power)
	r.ApparentPower = uint(e.Energy.ApparentPower)
	r.ReactivePower = uint(e.Energy.ReactivePower)
	r.Voltage = uint(e.Energy.Voltage)
	r.Current = e.Energy.Current

	return r
}
