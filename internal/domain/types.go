package domain

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

type PurgeJob struct {
	ChannelID  string    `json:"channel_id"`
	GuildID    string    `json:"guild_id"`
	Mode       string    `json:"mode"`        // "delayed" or "recurring"
	DelayUntil time.Time `json:"delay_until"` // relevant only for "delayed"
	OlderThan  string    `json:"older_than"`  // relevant only for "recurring"
	StartedAt  time.Time `json:"started_at"`
	Silent     bool      `json:"silent"`
}

type ShortLink struct {
	ShortID  string    `json:"short_id"`
	Original string    `json:"original"`
	UserID   string    `json:"user_id"`
	Created  time.Time `json:"created"`
	Clicks   int       `json:"clicks"`
}

type Task struct {
	UserID     string    `json:"user_id"`
	MessageID  string    `json:"task_message_id"`
	AssignedAt time.Time `json:"assigned_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Status     string    `json:"status"` // "pending", "completed", "failed", "safeword"
}

type Record struct {
	AnnounceChannel      string               `json:"announce_channel"`
	ConfessChannel       string               `json:"confess_channel"`
	CommandsDisabled     []string             `json:"commands_disabled"`
	CommandsHistory      []CommandHistory     `json:"commands_history"`
	CommandHashes        map[string]string    `json:"command_hashes,omitempty"` // slash command name -> hash for sync
	DisciplineRoles      map[string]string    `json:"discipline_roles"`
	MediaCategories      []string             `json:"media_categories"`
	MediaDefault         string               `json:"media_default"`
	PurgeJobs            map[string]PurgeJob  `json:"purge_jobs"` // key = channelID
	ShortLinks           []ShortLink          `json:"short_links"`
	TaskCooldowns        map[string]time.Time `json:"task_cooldowns"`
	TaskList             map[string]Task      `json:"task_list"`
	TaskRole             string               `json:"task_role"`
	TranslateChannels    []string             `json:"translate_channels"`
	MusicPlaybackHistory []MusicPlayback      `json:"music_playback_history,omitempty"`
	NextMusicHistoryID   uint64               `json:"next_music_history_id"`
}

type MusicPlayback struct {
	ID               uint64    `json:"id"`
	PlayedAt         time.Time `json:"played_at"`
	URL              string    `json:"url"`
	Title            string    `json:"title"`
	CurrentParser    string    `json:"current_parser"`
	AvailableParsers []string  `json:"available_parsers"`
	SourceName       string    `json:"source_name"`
}
