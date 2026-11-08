package config

import (
	"fmt"
	"log/slog"
	"time"

	envconfig "github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type CorsConfig struct {
	Allowed bool   `env:"ALLOWED"`
	Host    string `env:"HOST"`
}

type JwtConfig struct {
	Secret string `env:"SECRET"`
}

type GoogleOauth struct {
	ClientID      string   `env:"CLIENT_ID"`
	AllowedEmails []string `env:"ALLOWED_EMAILS" envSeparator:","`
}

type WebConfig struct {
	Host  string       `env:"HOST"`
	Cors  *CorsConfig  `env:",init" envPrefix:"CORS_"`
	Oauth *GoogleOauth `env:",init" envPrefix:"OAUTH_"`
	Jwt   *JwtConfig   `env:",init" envPrefix:"JWT_"`
}

type MqttConfig struct {
	DSN          string `env:"DSN,required"`
	User         string `env:"USER"`
	Pass         string `env:"PASS"`
	ClientId     string `env:"CLIENT_ID"`
	CleanSession bool   `env:"CLEAN_SESSION"`
}

type StorageConfig struct {
	Driver      string        `env:"DRIVER,required"`
	DSN         string        `env:"DSN,required"`
	FlushPeriod time.Duration `env:"FLUSH_PERIOD"`
}

type Config struct {
	Web      *WebConfig      `env:",init" envPrefix:"WEB_"`
	Mqtt     *MqttConfig     `env:",init" envPrefix:"MQTT_"`
	Storage  *StorageConfig  `env:",init" envPrefix:"STORAGE_"`
	Telegram *TelegramConfig `env:",init" envPrefix:"TELEGRAM_"`
	Logger   *LoggerConfig   `env:",init" envPrefix:"LOG_"`
}

type LoggerConfig struct {
	Level slog.Level `env:"LEVEL" envDefault:"INFO"`
}

type TelegramConfig struct {
	BotToken string `env:"BOT_TOKEN"`
	ChatID   string `env:"CHAT_ID"`
}

func Load() (*Config, error) {
	_ = godotenv.Load("../.env") // ignore error if no .env (prod)

	var config Config
	if err := envconfig.Parse(&config); err != nil {
		panic(err)
	}

	if config.Storage.Driver != "sqlite" {
		return nil, fmt.Errorf("Unknown driver %s", config.Storage.Driver)
	}

	return &config, nil
}
