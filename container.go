package main

import (
	"smart-home/config"
	"smart-home/mqtt"
	"smart-home/web"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Container struct {
	Mqtt *mqtt.MqttConsumer
	Web  *web.Server
}

func BuildContainer(cfg *config.Config) (*Container, error) {
	db, err := gorm.Open(sqlite.Open(cfg.Storage.DSN), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	storage, err := mqtt.NewStorage(db, cfg)
	if err != nil {
		return nil, err
	}

	mqttClient, err := mqtt.NewMqtt(cfg)
	if err != nil {
		return nil, err
	}

	mqtt := mqtt.NewMqttConsumer(storage, mqttClient)

	sensorsCtl := web.NewSensorsController(db, storage)
	devicesCtl := web.NewDevicesController(db)
	router := chi.NewMux()
	webServer := web.NewWebServer(router, sensorsCtl, devicesCtl)

	c := Container{
		Mqtt: mqtt,
		Web:  webServer,
	}

	return &c, nil
}
