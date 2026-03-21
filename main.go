package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"smart-home/config"
	"smart-home/container"
	"syscall"

	"github.com/urfave/cli/v3"
)

const (
	ExitErr = 1
	ExitOk  = 0
)

func main() {
	config.InitLogger()
	slog.Info("Starting")
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Config error", "err", err)
		os.Exit(ExitErr)
	}
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
				Name:  "run",
				Usage: "",
				Action: func(ctx context.Context, _ *cli.Command) error {
					if err := container.Mqtt.Run(); err != nil {
						return err
					}

					defer container.Mqtt.Shutdown()
					defer container.Storage.Shutdown()
					defer container.EventHandler.Shutdown()

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
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		slog.Error("Command error", "err", err)
		os.Exit(ExitErr)
	}

	slog.Info("Exiting")
}
