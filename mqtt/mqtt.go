package mqtt

import (
	"log/slog"
	"smart-home/config"
	"time"

	driver "github.com/eclipse/paho.mqtt.golang"
)

var mqttMsgChan = make(chan driver.Message)

var messagePubHandler driver.MessageHandler = func(client driver.Client, msg driver.Message) {
	mqttMsgChan <- msg
}

var connectLostHandler driver.ConnectionLostHandler = func(client driver.Client, err error) {
	slog.Error("Connection lost", "err", err)
}

var reconnectingHandler = func(c driver.Client, options *driver.ClientOptions) {
	slog.Warn("...... mqtt reconnecting ......")
}

func NewMqtt(cfg *config.Config, consumer *Consumer) driver.Client {
	opts := driver.NewClientOptions().
		AddBroker(cfg.Mqtt.DSN).
		SetClientID(cfg.Mqtt.ClientId).
		SetOrderMatters(false).                      // allows async
		SetDefaultPublishHandler(messagePubHandler). // consumes afer connection corrupted
		SetOnConnectHandler(consumer.Subscribe).
		SetConnectionLostHandler(connectLostHandler).
		SetReconnectingHandler(reconnectingHandler).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(30 * time.Second).
		SetUsername(cfg.Mqtt.User).
		SetPassword(cfg.Mqtt.Pass).
		SetCleanSession(true) // experimental

	client := driver.NewClient(opts)

	return client
}
