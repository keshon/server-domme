// /pkg/config/config.go
package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, falling back to system environment variables")
	}
}

func Get(key string) string {
	return os.Getenv(key)
}

type Config struct {
	DiscordToken string
	StoragePath  string
}

func New() *Config {
	var storagePath string

	if Get("STORAGE_PATH") == "" {
		storagePath = "datastore.json"
	} else {
		storagePath = Get("STORAGE_PATH")
	}

	if Get("DISCORD_TOKEN") == "" {
		log.Fatal("DISCORD_TOKEN is not set")
	}

	return &Config{
		DiscordToken: Get("DISCORD_TOKEN"),
		StoragePath:  storagePath,
	}
}
