package container

import (
	"smart-home/config"
	"smart-home/event"
	"smart-home/internal"
	"smart-home/internal/handler"
	"smart-home/mqtt"
	"smart-home/web"

	driver "github.com/eclipse/paho.mqtt.golang"
	"github.com/go-chi/jwtauth/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Container struct {
	MqttClient    driver.Client // interface
	MqttPublisher *mqtt.Publisher
	StateMonitor  *internal.StateMonitor
	DeviceMap     *internal.DeviceMap
	Web           *web.Server
	Storage       *internal.Storage
	EventHandler  *mqtt.EventParser
	Auth          *jwtauth.JWTAuth
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
	states := internal.NewDeviceStateManager(devices)
	storage, err := internal.NewStorage(db, cfg, states)
	if err != nil {
		return nil, err
	}

	dispatcher := event.NewDispatcher()
	eventParser := mqtt.NewEventParser(dispatcher)

	consumer := mqtt.NewMqttConsumer(states, eventParser)
	mqttClient := mqtt.NewMqtt(cfg, consumer)
	publisher := mqtt.NewPublisher(mqttClient, states)
	stateMonitor := internal.NewStateMonitor(dispatcher, states)

	eventHandler := handler.NewEventHandler(states, publisher, storage)
	eventHandler.Subscribe(dispatcher, states)

	ws := web.NewWebSocketServer()
	ws.Subscribe(dispatcher, states)

	tokenAuth := jwtauth.New("HS256", []byte(cfg.Web.Jwt.Secret), nil)

	authCtl := web.NewAuthController(cfg.Web.Oauth, tokenAuth)
	sensorsCtl := web.NewSensorsController(db, storage)
	dailyCtl := web.NewSensorsDailyController(db, states)
	configurableCtl := web.NewSensorsConfigurableController(db, states, storage)
	devicesCtl := web.NewDevicesController(states)
	deviceControlCtl := web.NewDeviceControlController(publisher, states, config.GetNewTimezones())

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
		MqttClient:    mqttClient,
		MqttPublisher: publisher,
		StateMonitor:  stateMonitor,
		DeviceMap:     deviceMap,
		Web:           webServer,
		Storage:       storage,

		Auth: tokenAuth,
	}

	return &c, nil
}
