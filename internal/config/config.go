package config

import (
	"fmt"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config is the configuration for the bot.
type Config struct {
	DiscordToken          string   `env:"DISCORD_TOKEN,required"`
	DiscordGuildBlacklist []string `env:"DISCORD_GUILD_BLACKLIST" envSeparator:","`
	StoragePath           string   `env:"STORAGE_PATH" envDefault:"./data/store"` // directory the datastore owns (WAL + snapshots)
	DeveloperID           string   `env:"DEVELOPER_ID"`
	InitSlashCommands     bool     `env:"INIT_SLASH_COMMANDS" envDefault:"false"`

	// CommandTimeout is a hard timebox for command execution.
	CommandTimeout time.Duration `env:"COMMAND_TIMEOUT" envDefault:"30s"`
	// CommandParallelism limits concurrently running command handlers.
	CommandParallelism int `env:"COMMAND_PARALLELISM" envDefault:"16"`
	// WSSilenceTimeout triggers a session restart if no gateway messages are
	// received.
	WSSilenceTimeout time.Duration `env:"WS_SILENCE_TIMEOUT" envDefault:"2m"`

	// DiscordUnhealthyMode controls what happens when watchdogs/API probe decide
	// the session is unhealthy. Supported: restart-session|ignore.
	DiscordUnhealthyMode string `env:"DISCORD_UNHEALTHY_MODE" envDefault:"restart-session"`
	// DiscordUnhealthyGrace allows ignoring the first N unhealthy signals within
	// DiscordUnhealthyWindow before triggering a session restart. Applies to
	// mode=restart only.
	DiscordUnhealthyGrace int `env:"DISCORD_UNHEALTHY_GRACE" envDefault:"0"`
	// DiscordUnhealthyWindow is the counting window for DiscordUnhealthyGrace.
	DiscordUnhealthyWindow time.Duration `env:"DISCORD_UNHEALTHY_WINDOW" envDefault:"1m"`

	// Logging (applog / zerolog). LOG_FILE empty = stderr only (pretty console).
	LogLevel      string `env:"LOG_LEVEL" envDefault:"info"`
	LogFile       string `env:"LOG_FILE"`
	LogMaxSizeMB  int    `env:"LOG_MAX_SIZE_MB" envDefault:"10"`
	LogMaxBackups int    `env:"LOG_MAX_BACKUPS" envDefault:"3"`
	LogMaxAgeDays int    `env:"LOG_MAX_AGE_DAYS" envDefault:"0"`
	LogCompress   bool   `env:"LOG_COMPRESS" envDefault:"false"`

	TasksPath        string   `env:"TASKS_PATH,required"`
	ProtectedUsers   []string `env:"PROTECTED_USERS" envSeparator:","`
	ShortLinkBaseURL string   `env:"SHORTLINK_BASE_URL"`
	// ShortLinkAddr is the listen address for the redirect server (host:port).
	ShortLinkAddr string `env:"SHORTLINK_ADDR" envDefault:":8787"`
	// HealthCheckPath registers a shallow GET/HEAD health endpoint at this path
	// (empty = disabled).
	HealthCheckPath string `env:"HEALTHCHECK_PATH" envDefault:"/ping"`

	// Chat persona. Off by default, and deliberately so: turning it on sends
	// the contents of opted-in channels to third-party relays to produce
	// replies, which is a different privacy posture from everything else this
	// bot does. Channels are opted in one at a time on top of this — see
	// /chat here.
	ChatEnabled bool `env:"CHAT_ENABLED" envDefault:"false"`
	// ChatNames is every spelling she answers to, comma separated, most
	// canonical first: "ServerDomme,Server-Domme,Server Domme".
	//
	// A list rather than a single name because there is no one answer. The
	// account username, the per-guild nickname and whatever an operator put
	// here can all differ, and Discord renders a mention as the account
	// username — so a bot told it is only "Dev" reads "@DevBot" and decides
	// that is someone else. The names Discord reports are added to this list
	// at runtime; see chat.Service.namesFor.
	ChatNames []string `env:"CHAT_NAME" envSeparator:"," envDefault:"Domme"`
	// ChatCharacterPath is the authored character file. See internal/mind.
	ChatCharacterPath string `env:"CHAT_CHARACTER_PATH" envDefault:"./data/character.md"`

	// ChatUsePollinations and ChatUseG4F select the free public backends.
	// Pollinations is off by default because anonymously it refuses a prompt
	// this size with 402 KEY_BUDGET_EXHAUSTED — see ai.Options. Point
	// CHAT_BASE_URL at it with a key if you have one.
	ChatUsePollinations bool `env:"CHAT_USE_POLLINATIONS" envDefault:"false"`
	ChatUseG4F          bool `env:"CHAT_USE_G4F" envDefault:"true"`
	// ChatG4FPicks is how many g4f.space models to keep in the pool. Each one
	// is on a different donated server, so it is really a count of independent
	// machines to fall back through.
	ChatG4FPicks int `env:"CHAT_G4F_PICKS" envDefault:"3"`
	// ChatG4FAPIKey authenticates to the g4f relay. Without one its anonymous
	// allowance is proof-of-work earned per IP in a browser, which a server
	// never has — every call then comes back 402 insufficient_credits.
	ChatG4FAPIKey string `env:"CHAT_G4F_API_KEY"`

	// ChatBaseURL, ChatModel and ChatAPIKey point at any other
	// OpenAI-compatible endpoint — a local Ollama or LM Studio, or a paid API.
	// Set when the free relays are not good enough for the character.
	ChatBaseURL string `env:"CHAT_BASE_URL"`
	ChatModel   string `env:"CHAT_MODEL"`
	ChatAPIKey  string `env:"CHAT_API_KEY"`

	// Chat*Chance are the odds she answers each kind of approach, 0 to 1.
	// Someone who answers every single time is recognisably a machine, so the
	// defaults are short of certainty; set them to 1 to take the choice away.
	ChatMentionChance float64 `env:"CHAT_MENTION_CHANCE" envDefault:"0.88"`
	ChatNamedChance   float64 `env:"CHAT_NAMED_CHANCE" envDefault:"0.4"`
	ChatReplyChance   float64 `env:"CHAT_REPLY_CHANCE" envDefault:"0.92"`
	// ChatFollowUpChance is the odds she answers the next untagged message
	// from whoever she is already talking to. Set it to 0 to require a tag,
	// a name or a reply every single time.
	ChatFollowUpChance float64 `env:"CHAT_FOLLOWUP_CHANCE" envDefault:"0.8"`
}

// IsDeveloper reports whether userID is the configured developer (avoids
// discord import in middleware).
//
// Both ids must be non-empty. Comparing them alone is not enough: DEVELOPER_ID
// is unset on most deployments, and an event carrying no user id would then
// match it and be treated as the developer — which since the permission
// middleware started honouring this is a bypass of every admin gate the bot
// has. Fails closed when either side is missing.
func IsDeveloper(cfg *Config, userID string) bool {
	if cfg == nil || cfg.DeveloperID == "" || userID == "" {
		return false
	}
	return cfg.DeveloperID == userID
}

// New returns a new Config.
func NewConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "No .env file found, falling back to system environment variables")
	}

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
