package container

import (
	"smart-home/config"
	"smart-home/internal"
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
	StateMonitor  *mqtt.StateMonitor
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

	ws := web.NewWebSocketServer()
	eventHandler := mqtt.NewEventHandler(storage, ws, states)
	consumer := mqtt.NewMqttConsumer(states, eventHandler)
	mqttClient := mqtt.NewMqtt(cfg, consumer)
	publisher := mqtt.NewPublisher(mqttClient, states)
	stateMonitor := mqtt.NewStateMonitor(ws, states)

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
		MqttClient:    mqttClient,
		MqttPublisher: publisher,
		StateMonitor:  stateMonitor,
		DeviceMap:     deviceMap,
		Web:           webServer,
		Storage:       storage,
		EventHandler:  eventHandler,
	}

	return &c, nil
}
