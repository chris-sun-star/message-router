package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Server struct {
		Port string `yaml:"port"`
	} `yaml:"server"`

	Database struct {
		DSN string `yaml:"dsn"`
	} `yaml:"database"`

	JWT struct {
		Secret string `yaml:"secret"`
	} `yaml:"jwt"`

	Encryption struct {
		Key string `yaml:"key"`
	} `yaml:"encryption"`

	Channels struct {
		Telegram struct {
			APIID   int    `yaml:"api_id"`
			APIHash string `yaml:"api_hash"`
		} `yaml:"telegram"`
	} `yaml:"channels"`

	Network struct {
		Proxy string `yaml:"proxy"`
	} `yaml:"network"`
}

var AppConfig *Config

func LoadConfig(path string) error {
	file, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading config file: %v", err)
	}

	var cfg Config
	err = yaml.Unmarshal(file, &cfg)
	if err != nil {
		return fmt.Errorf("error unmarshaling config: %v", err)
	}

	AppConfig = &cfg
	return nil
}
