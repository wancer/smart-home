package config

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	envconfig "github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	privateauth "github.com/wancer/go-private-auth"
)

type CorsConfig struct {
	Allowed bool   `env:"ALLOWED"`
	Host    string `env:"HOST"`
}

type JwtConfig struct {
	Secret string        `env:"SECRET,required"`
	TTL    time.Duration `env:"TTL,required,notEmpty"` // e.g. 720h; max 30 days
}

type GoogleOauth struct {
	ClientID string `env:"CLIENT_ID,required"`
	// AllowedSubs and AllowedEmails are parallel lists: AllowedSubs[i] is the
	// Google account ID ("sub") of the account whose email is AllowedEmails[i].
	AllowedSubs   []string `env:"ALLOWED_SUBS,required" envSeparator:","`
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
	if envFile := os.Getenv("ENVFILE"); envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			return nil, fmt.Errorf("invalid .env file: %w", err)
		}
	}

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

// validate checks the auth config with the shared library's rules.
func (c *WebConfig) validate() error {
	authCfg, err := c.AuthConfig()
	if err != nil {
		return err
	}
	return authCfg.Validate()
}

// AuthConfig maps this service's config onto the shared auth library.
func (c *WebConfig) AuthConfig() (privateauth.Config, error) {
	users, err := privateauth.PairUsers(c.Oauth.AllowedSubs, c.Oauth.AllowedEmails)
	if err != nil {
		return privateauth.Config{}, fmt.Errorf("WEB_OAUTH_ALLOWED_SUBS / WEB_OAUTH_ALLOWED_EMAILS: %w", err)
	}
	return privateauth.Config{
		Issuer:         "smart-home",
		Audience:       "smart-home-web",
		Secret:         c.Jwt.Secret,
		TTL:            c.Jwt.TTL,
		GoogleClientID: c.Oauth.ClientID,
		Allowed:        users,
	}, nil
}
