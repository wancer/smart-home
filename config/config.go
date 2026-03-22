package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"
)

type CorsConfig struct {
	Allowed bool   `yaml:"allowed"`
	Host    string `yaml:"host"`
}

type JwtConfig struct {
	Secret string `yaml:"secret"`
}

type GoogleOauth struct {
	RedirectURL   string   `yaml:"redirectUrl"`
	ClientSecret  string   `yaml:"secret"`
	AllowedEmails []string `yaml:"allowedEmails"`
}

type WebConfig struct {
	Host  string       `yaml:"host"`
	Cors  *CorsConfig  `yaml:"cors"`
	Oauth *GoogleOauth `yaml:"oauth"`
	Jwt   *JwtConfig   `yaml:"jwt"`
}

type MqttConfig struct {
	DSN      string `yaml:"dsn"`
	User     string `yaml:"user"`
	Pass     string `yaml:"pass"`
	ClientId string `yaml:"clientId"`
}

type Device struct {
	Name  string `yaml:"name"`
	Topic string `yaml:"topic"`
}

type StorageConfig struct {
	Driver      string        `yaml:"driver"`
	DSN         string        `yaml:"dsn"`
	FlushPeriod time.Duration `yaml:"flushPeriod"`
}

type Config struct {
	Web     *WebConfig     `yaml:"web"`
	Mqtt    *MqttConfig    `yaml:"mqtt"`
	Devices []Device       `yaml:"devices"`
	Storage *StorageConfig `yaml:"storage"`
	Logger  *LoggerConfig  `yaml:"logger"`
}

type LoggerConfig struct {
	Level slog.Level `yaml:"level"`
}

func Load() (*Config, error) {
	filename, _ := filepath.Abs("./config.yaml")
	yamlFile, err := os.ReadFile(filename)
	if err != nil {
		panic(err)
	}

	var config Config
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}

	if config.Storage.Driver != "sqlite" {
		return nil, fmt.Errorf("Unknown driver %s", config.Storage.Driver)
	}

	return &config, nil
}
