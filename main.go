package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"smart-home/config"
	"smart-home/container"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"
)

const (
	ExitErr = 1
	ExitOk  = 0
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	config.InitLogger(cfg)
	slog.Info("Config loaded")

	container, err := container.Build(cfg)
	if err != nil {
		slog.Error("Config error", "err", err)
		os.Exit(ExitErr)
	}
	slog.Info("Container built")

	app := &cli.Command{
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "",
				Action: func(ctx context.Context, _ *cli.Command) error {
					if token := container.MqttClient.Connect(); token.Wait() && token.Error() != nil {
						return token.Error()
					}

					// ToDo: fix race condition when subscribe not yet finised
					time.Sleep(2 * time.Second)
					container.MqttPublisher.PublishAllStates()
					monitorStop := container.StateMonitor.Run()

					defer close(monitorStop)
					defer container.MqttClient.Disconnect(10_000) // 10s == 10k ms
					defer container.Storage.Shutdown()

					var err error
					go func() {
						err = container.Web.Start(ctx)
					}()

					quit := make(chan os.Signal, 1)
					signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
					<-quit

					return err
				},
			},
			{
				Name: "get-auth",
				Action: func(_ context.Context, _ *cli.Command) error {
					_, tokenString, _ := container.Auth.Encode(map[string]interface{}{"user_id": 123})
					fmt.Sprintf("DEBUG: a sample jwt is %s\n\n", tokenString)
					return nil
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		slog.Error("Command error", "err", err)
		os.Exit(ExitErr)
	}

	slog.Info("Exiting")
}
