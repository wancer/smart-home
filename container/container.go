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
	privateauth "github.com/wancer/go-private-auth"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Container struct {
	MqttClient   driver.Client // interface
	StateMonitor *internal.StateMonitor
	Firmware     *internal.FirmwareChecker
	Telegram     *internal.TelegramNotifier
	Notifier     *automation.Notifier
	Web          *web.Server
	Storage      *internal.Storage
	Auth         *privateauth.Auth
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

	states := internal.NewDeviceStateManager(devices)
	storage, err := internal.NewStorage(db, cfg, states)
	if err != nil {
		return nil, err
	}

	dispatcher := event.NewDispatcher()
	storage.OnSeriesChange = func(deviceId uint) {
		dispatcher.Dispatch(&event.SeriesChanged{DeviceId: deviceId})
	}
	eventParser := mqtt.NewEventParser(dispatcher)

	consumer := mqtt.NewMqttConsumer(states, eventParser)
	mqttClient := mqtt.NewMqtt(cfg, consumer)
	publisher := mqtt.NewPublisher(mqttClient)
	stateMonitor := internal.NewStateMonitor(dispatcher, states)
	firmwareChecker := internal.NewFirmwareChecker()

	telegram := internal.NewTelegramNotifier(cfg.Telegram.BotToken, cfg.Telegram.ChatID)
	notifier := automation.NewNotifier(db, states, storage, telegram)
	if err := notifier.Reload(); err != nil {
		return nil, err
	}
	notifier.Subscribe(dispatcher)

	irRunner := automation.NewIrRuleRunner(db, states, publisher)
	profiles := internal.NewDeviceProfiles(db)
	eventHandler := handler.NewEventHandler(dispatcher, states, publisher, storage, profiles, irRunner)
	eventHandler.Subscribe()

	ws := web.NewWebSocketServer()
	ws.Subscribe(dispatcher, states, storage) // after the handlers: reads their live values

	authCfg, err := cfg.Web.AuthConfig()
	if err != nil {
		return nil, err
	}
	auth, err := privateauth.New(authCfg)
	if err != nil {
		return nil, err
	}

	sensorsCtl := web.NewSensorsController(db, storage, states)
	dailyCtl := web.NewSensorsDailyController(db, states)
	configurableCtl := web.NewSensorsConfigurableController(db, states, storage)
	devicesCtl := web.NewDevicesController(states, db, storage, consumer, publisher, notifier)
	deviceControlCtl := web.NewDeviceControlController(publisher, states, config.GetNewTimezones(), firmwareChecker, profiles)
	deviceTimersCtl := web.NewDeviceTimersController(publisher, states)
	deviceRulesCtl := web.NewDeviceRulesController(publisher, states)
	irCtl := web.NewIrController(db, states, publisher)
	irRulesCtl := web.NewIrRulesController(db, states, irRunner)
	notificationsCtl := web.NewNotificationsController(db, states, storage, notifier, telegram)

	webServer := web.NewWebServer(
		cfg.Web,
		auth,
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
	)

	c := Container{
		MqttClient:   mqttClient,
		StateMonitor: stateMonitor,
		Firmware:     firmwareChecker,
		Telegram:     telegram,
		Notifier:     notifier,
		Web:          webServer,
		Storage:      storage,
		Auth:         auth,
	}

	return &c, nil
}
