package config

import (
	"log/slog"
	"os"
	"time"
)

func InitLogger(cfg *Config) {
	logger := slog.New(
		slog.NewJSONHandler(
			os.Stdout,
			&slog.HandlerOptions{
				Level: cfg.Logger.Level,
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey {
						if t, ok := a.Value.Any().(time.Time); ok {
							a.Value = slog.StringValue(t.Format(time.DateTime))
						}
					}
					return a
				},
			},
		),
	)
	slog.SetDefault(logger)
}
