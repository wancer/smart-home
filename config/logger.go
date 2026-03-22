package config

import (
	"log/slog"
	"os"
)

func InitLogger(cfg *Config) {
	logger := slog.New(
		slog.NewJSONHandler(
			os.Stdout,
			&slog.HandlerOptions{Level: cfg.Logger.Level},
		),
	)
	slog.SetDefault(logger)
}
