package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"smart-home/config"
	"syscall"

	"github.com/urfave/cli/v3"
)

const (
	ExitErr = 1
	ExitOk  = 0
)

func main() {
	slog.Info("Starting")
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Config error", "err", err)
		os.Exit(ExitErr)
	}
	slog.Info("Config loaded")
	container, err := BuildContainer(cfg)
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
					if err := container.Mqtt.Run(cfg); err != nil {
						return err
					}
					defer container.Mqtt.Shutdown()

					// Gracefully shut down server
					// ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Second)
					// defer cancel()

					var err error
					go func() {
						err = container.Web.Start(cfg.WebHost, ctx)
					}()

					// Wait for interrupt signal
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
