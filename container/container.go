package container

import (
	"smart-home/config"
	"smart-home/internal"
	"smart-home/mqtt"
	"smart-home/web"

	"github.com/go-chi/jwtauth/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Container struct {
	MqttConsumer  *mqtt.Consumer
	MqttPublisher *mqtt.Publisher
	DeviceMap     *internal.DeviceMap
	Web           *web.Server
	Storage       *internal.Storage
	EventHandler  *mqtt.EventHandler
}

func Build(cfg *config.Config) (*Container, error) {
	db, err := gorm.Open(sqlite.Open(cfg.Storage.DSN), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	init := NewDatabaseInitializer(db)
	if err := init.migrate(); err != nil {
		return nil, err
	}
	devices, err := init.syncDevices(cfg.Devices)
	if err != nil {
		return nil, err
	}

	deviceMap := internal.NewDeviceMap(devices)
	states := internal.NewDeviceStateStorage(devices)
	storage, err := internal.NewStorage(db, cfg, states)
	if err != nil {
		return nil, err
	}

	mqttClient, err := mqtt.NewMqtt(cfg)
	if err != nil {
		return nil, err
	}

	ws := web.NewWebSocketServer()

	eventHandler := mqtt.NewEventHandler(storage, ws, states)
	consumer := mqtt.NewMqttConsumer(mqttClient, states, eventHandler)
	publisher := mqtt.NewPublisher(mqttClient, states)

	tokenAuth := jwtauth.New("HS256", []byte(cfg.Web.Jwt.Secret), nil)
	sensorsCtl := web.NewSensorsController(db, storage)
	dailyCtl := web.NewSensorsDailyController(db, states)
	configurableCtl := web.NewSensorsConfigurableController(db, states, storage)
	devicesCtl := web.NewDevicesController(states)
	deviceControlCtl := web.NewDeviceControlController(publisher, states, config.GetNewTimezones())
	authCtl := web.NewAuthController(cfg.Web.Oauth, tokenAuth)

	webServer := web.NewWebServer(
		cfg.Web,
		tokenAuth,
		ws,
		sensorsCtl,
		dailyCtl,
		configurableCtl,
		devicesCtl,
		deviceControlCtl,
		authCtl,
	)

	c := Container{
		MqttConsumer:  consumer,
		MqttPublisher: publisher,
		DeviceMap:     deviceMap,
		Web:           webServer,
		Storage:       storage,
		EventHandler:  eventHandler,
	}

	return &c, nil
}
