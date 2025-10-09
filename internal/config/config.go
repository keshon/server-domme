package config

import (
	"log"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config is the configuration for the bot.
type Config struct {
	DiscordToken      string   `env:"DISCORD_TOKEN,required"`
	StoragePath       string   `env:"STORAGE_PATH" envDefault:"./data/datastore.json"`
	TasksPath         string   `env:"TASKS_PATH,required"`
	ProtectedUsers    []string `env:"PROTECTED_USERS" envSeparator:","`
	DeveloperID       string   `env:"DEVELOPER_ID"`
	InitSlashCommands bool     `env:"INIT_SLASH_COMMANDS" envDefault:"false"`
	AIProvider        string   `env:"AI_PROVIDER"`
	AIPromtPath       string   `env:"AI_PROMPT_PATH"`
}

// New returns a new Config.
func New() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, falling back to system environment variables")
	}

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	return &cfg
}
