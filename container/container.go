package container

import (
	"smart-home/automation"
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
	Firmware      *internal.FirmwareChecker
	Telegram      *internal.TelegramNotifier
	Notifier      *automation.Notifier
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
	devices, err := init.loadDevices()
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
	publisher := mqtt.NewPublisher(mqttClient)
	stateMonitor := internal.NewStateMonitor(dispatcher, states)
	firmwareChecker := internal.NewFirmwareChecker()

	telegram := internal.NewTelegramNotifier(cfg.Telegram.BotToken, cfg.Telegram.ChatID)
	notifier := automation.NewNotifier(db, states, telegram)
	if err := notifier.Reload(); err != nil {
		return nil, err
	}
	notifier.Subscribe(dispatcher)

	irRunner := automation.NewIrRuleRunner(db, states, publisher)
	eventHandler := handler.NewEventHandler(dispatcher, states, publisher, storage, irRunner)
	eventHandler.Subscribe(dispatcher, states)

	ws := web.NewWebSocketServer()
	ws.Subscribe(dispatcher, states)

	tokenAuth := jwtauth.New("HS256", []byte(cfg.Web.Jwt.Secret), nil)

	authCtl := web.NewAuthController(cfg.Web.Oauth, tokenAuth)
	sensorsCtl := web.NewSensorsController(db, storage, states)
	dailyCtl := web.NewSensorsDailyController(db, states)
	configurableCtl := web.NewSensorsConfigurableController(db, states, storage)
	devicesCtl := web.NewDevicesController(states, db, storage, consumer, publisher, notifier)
	deviceControlCtl := web.NewDeviceControlController(publisher, states, config.GetNewTimezones(), firmwareChecker)
	deviceTimersCtl := web.NewDeviceTimersController(publisher, states)
	deviceRulesCtl := web.NewDeviceRulesController(publisher, states)
	irCtl := web.NewIrController(db, states, publisher)
	irRulesCtl := web.NewIrRulesController(db, states, irRunner)
	notificationsCtl := web.NewNotificationsController(db, states, notifier, telegram)

	webServer := web.NewWebServer(
		cfg.Web,
		tokenAuth,
		ws,
		sensorsCtl,
		dailyCtl,
		configurableCtl,
		devicesCtl,
		deviceControlCtl,
		deviceTimersCtl,
		deviceRulesCtl,
		irCtl,
		irRulesCtl,
		notificationsCtl,
		authCtl,
	)

	c := Container{
		MqttClient:    mqttClient,
		MqttPublisher: publisher,
		StateMonitor:  stateMonitor,
		Firmware:      firmwareChecker,
		Telegram:      telegram,
		Notifier:      notifier,
		DeviceMap:     deviceMap,
		Web:           webServer,
		Storage:       storage,

		Auth: tokenAuth,
	}

	return &c, nil
}
