// /pkg/config/config.go
package config

import (
	"log"
	"os"
	"strings"

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
	DiscordToken      string
	StoragePath       string
	TasksPath         string
	ProtectedUsers    []string
	DeveloperID       string
	InitSlashCommands bool
}

func New() *Config {
	var storagePath string

	if Get("STORAGE_PATH") == "" {
		storagePath = "./data/datastore.json"
	} else {
		storagePath = Get("STORAGE_PATH")
	}

	if Get("DISCORD_TOKEN") == "" {
		log.Fatal("DISCORD_TOKEN is not set")
	}

	if Get("TASKS_PATH") == "" {
		log.Fatal("TASKS_PATH is not set")
	}

	protectedUsersEnv := Get("PROTECTED_USERS")
	var protectedUsers []string
	if protectedUsersEnv != "" {
		protectedUsers = strings.Split(protectedUsersEnv, ",")
		for i := range protectedUsers {
			protectedUsers[i] = strings.TrimSpace(protectedUsers[i])
		}
	}

	return &Config{
		DiscordToken:      Get("DISCORD_TOKEN"),
		StoragePath:       storagePath,
		TasksPath:         Get("TASKS_PATH"),
		ProtectedUsers:    protectedUsers,
		DeveloperID:       Get("DEVELOPER_ID"),
		InitSlashCommands: Get("INIT_SLASH_COMMANDS") == "true",
	}
}
