package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordToken string
	DiscordGuild string
	ServerPort   string
	AppEnv       string
}

var ConfigInstance *Config

func LoadConfig() (*Config, error) {
	err := godotenv.Load(".env")
	if err != nil {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	token := os.Getenv("DISCORD_TOKEN")
	guild := os.Getenv("DISCORD_GUILD")
	port := os.Getenv("SERVER_PORT")
	appEnv := os.Getenv("APP_ENV")

	if token == "" || port == "" {
		return nil, fmt.Errorf("missing configuration values")
	}

	ConfigInstance = &Config{
		DiscordToken: token,
		DiscordGuild: guild,
		ServerPort:   port,
		AppEnv:       appEnv,
	}

	return ConfigInstance, nil
}
