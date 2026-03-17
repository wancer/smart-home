package config

import (
	"log/slog"
	"os"
)

func InitLogger(_ *Config) {
	logger := slog.New(
		slog.NewJSONHandler(
			os.Stdout,
			&slog.HandlerOptions{Level: slog.LevelInfo},
		),
	)
	slog.SetDefault(logger)
}
