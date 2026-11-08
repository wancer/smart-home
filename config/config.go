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
	Secret string        `env:"SECRET,required"`
	TTL    time.Duration `env:"TTL" envDefault:"1h"`
}

const (
	MinJwtSecretLen = 32
	DefaultJwtTTL   = time.Hour
	MaxJwtTTL       = 256 * time.Hour
)

type GoogleOauth struct {
	ClientID string `env:"CLIENT_ID,required"`
	// AllowedSubs is the primary whitelist (Google's stable account ID).
	AllowedSubs []string `env:"ALLOWED_SUBS,required" envSeparator:","`
	// AllowedEmails is a second check: the token's email must also be listed.
	AllowedEmails []string `env:"ALLOWED_EMAILS,required" envSeparator:","`
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
		return nil, err
	}

	if err := config.Web.validate(); err != nil {
		return nil, err
	}

	if config.Storage.Driver != "sqlite" {
		return nil, fmt.Errorf("Unknown driver %s", config.Storage.Driver)
	}

	return &config, nil
}

func (c *WebConfig) validate() error {
	if len(c.Jwt.Secret) < MinJwtSecretLen {
		return fmt.Errorf("WEB_JWT_SECRET must be at least %d bytes (generate with: openssl rand -base64 48)", MinJwtSecretLen)
	}
	if c.Jwt.TTL <= 0 || c.Jwt.TTL > MaxJwtTTL {
		return fmt.Errorf("WEB_JWT_TTL must be between 0 and 24h, got %s", c.Jwt.TTL)
	}
	if len(c.Oauth.AllowedSubs) == 0 {
		return fmt.Errorf("WEB_OAUTH_ALLOWED_SUBS must list at least one Google account sub")
	}
	if len(c.Oauth.AllowedEmails) == 0 {
		return fmt.Errorf("WEB_OAUTH_ALLOWED_EMAILS must list at least one email")
	}
	return nil
}
