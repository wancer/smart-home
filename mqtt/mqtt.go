package mqtt

import (
	"smart-home/config"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func NewMqtt(cfg *config.Config) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Mqtt.DSN)
	opts.SetClientID(cfg.Mqtt.ClientId)
	opts.SetOrderMatters(false)
	opts.SetAutoReconnect(true)
	opts.SetDefaultPublishHandler(messagePubHandler)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	opts.Username = cfg.Mqtt.User
	opts.Password = cfg.Mqtt.Pass

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return client, nil
}
