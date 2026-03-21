package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"
)

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

type GoogleOauth struct {
	RedirectURL   string   `yaml:"redirectUrl"`
	ClientSecret  string   `yaml:"secret"`
	AllowedEmails []string `yaml:"allowedEmails"`
}

type Config struct {
	WebHost string        `yaml:"host"`
	Mqtt    MqttConfig    `yaml:"mqtt"`
	Devices []Device      `yaml:"devices"`
	Storage StorageConfig `yaml:"storage"`
	Oauth   *GoogleOauth  `yaml:"oauth"`
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
