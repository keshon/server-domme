package storagetypes

import (
	"time"
)

type CommandHistory struct {
	ChannelID   string    `json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	GuildName   string    `json:"guild_name"`
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Command     string    `json:"command"`
	Datetime    time.Time `json:"datetime"`
}

type Task struct {
	UserID     string    `json:"user_id"`
	MessageID  string    `json:"task_message_id"`
	AssignedAt time.Time `json:"assigned_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Status     string    `json:"status"` // "pending", "completed", "failed", "safeword"
}

type PurgeJob struct {
	ChannelID  string    `json:"channel_id"`
	GuildID    string    `json:"guild_id"`
	Mode       string    `json:"mode"`        // "delayed" or "recurring"
	DelayUntil time.Time `json:"delay_until"` // relevant only for "delayed"
	OlderThan  string    `json:"older_than"`  // relevant only for "recurring"
	StartedAt  time.Time `json:"started_at"`
	Silent     bool      `json:"silent"`
}

type Record struct {
	AnnounceChannel   string               `json:"announce_channel"`
	ConfessChannel    string               `json:"confess_channel"`
	CommandsDisabled  []string             `json:"commands_disabled"`
	CommandsHistory   []CommandHistory     `json:"commands_history"`
	DisciplineRoles   map[string]string    `json:"discipline_roles"`
	PurgeJobs         map[string]PurgeJob  `json:"purge_jobs"` // key = channelID
	TaskCooldowns     map[string]time.Time `json:"task_cooldowns"`
	TaskList          map[string]Task      `json:"task_list"`
	TaskRole          string               `json:"task_role"`
	TranslateChannels []string             `json:"translate_channels"`
}
