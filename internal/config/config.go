package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Telegram TelegramConfig `yaml:"telegram"`
	Router   RouterConfig   `yaml:"router"`
	Storage  StorageConfig  `yaml:"storage"`
}

type StorageConfig struct {
	FavoritesFile string `yaml:"favorites_file"`
}

type TelegramConfig struct {
	BotToken           string  `yaml:"bot_token"`
	AllowedUsers       []int64 `yaml:"allowed_users"`
	NotificationChatID int64   `yaml:"notification_chat_id"`
}

type RouterConfig struct {
	URL                 string `yaml:"url"`
	Login               string `yaml:"login"`
	Password            string `yaml:"password"`
	PollIntervalSeconds int    `yaml:"poll_interval_seconds"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config yaml: %w", err)
	}

	if cfg.Router.URL == "" {
		cfg.Router.URL = "http://192.168.1.1"
	}
	if cfg.Router.Login == "" {
		cfg.Router.Login = "admin"
	}
	if cfg.Router.PollIntervalSeconds <= 0 {
		cfg.Router.PollIntervalSeconds = 15
	}
	if cfg.Storage.FavoritesFile == "" {
		cfg.Storage.FavoritesFile = "data/favorites.json"
	}

	return &cfg, nil
}
