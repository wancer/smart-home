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
	Mqtt         *mqtt.MqttConsumer
	DeviceMap    *internal.DeviceMap
	Web          *web.Server
	Storage      *internal.Storage
	EventHandler *mqtt.EventHandler
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
	states := internal.NewStateStorage(deviceMap)
	storage, err := internal.NewStorage(db, cfg, deviceMap)
	if err != nil {
		return nil, err
	}

	mqttClient, err := mqtt.NewMqtt(cfg)
	if err != nil {
		return nil, err
	}

	ws := web.NewWebSocketServer()

	eventHandler := mqtt.NewEventHandler(deviceMap, storage, ws, states)
	mqtt := mqtt.NewMqttConsumer(mqttClient, deviceMap, eventHandler)

	tokenAuth := jwtauth.New("HS256", []byte(cfg.Web.Jwt.Secret), nil)
	sensorsCtl := web.NewSensorsController(db, storage)
	devicesCtl := web.NewDevicesController(states)
	authCtl := web.NewAuthController(cfg.Web.Oauth, tokenAuth)

	webServer := web.NewWebServer(
		cfg.Web,
		tokenAuth,
		ws,
		sensorsCtl,
		devicesCtl,
		authCtl,
	)

	c := Container{
		Mqtt:         mqtt,
		DeviceMap:    deviceMap,
		Web:          webServer,
		Storage:      storage,
		EventHandler: eventHandler,
	}

	return &c, nil
}
