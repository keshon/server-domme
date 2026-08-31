package storage

import (
	"fmt"
	"time"
)

// Persisted record types. Each satisfies datastore.Entity via Key(), which is
// what addresses the record in the write-ahead log.
//
// Two key shapes appear here. Rows identified by something the guild already
// names — a channel, a member — key on "<guildID>:<id>", which is enough to
// keep guilds apart. Rows that are only ever appended key on
// "<guildID>:<zero-padded id>": the padding makes lexicographic key order equal
// chronological id order, which is what lets an index read return history
// oldest-first without re-sorting.

// guildRowKey builds the ordered composite key for a per-guild numbered row.
func guildRowKey(guildID string, id uint64) string {
	return fmt.Sprintf("%s:%020d", guildID, id)
}

// guildScopedKey builds the composite key for a per-guild row that a natural
// id already identifies.
func guildScopedKey(guildID, id string) string {
	return guildID + ":" + id
}

// GuildSettings holds the per-guild configuration that is read far more often
// than it is written. It is one row per guild by design: these fields are set
// from settings commands, so contention is not a concern and keeping them
// together means one read serves a whole command.
type GuildSettings struct {
	GuildID              string            `json:"guild_id"`
	AnnounceChannel      string            `json:"announce_channel,omitempty"`
	ConfessChannel       string            `json:"confess_channel,omitempty"`
	CommandsDisabled     []string          `json:"commands_disabled,omitempty"`
	DisciplineRoles      map[string]string `json:"discipline_roles,omitempty"`
	MediaCategories      []string          `json:"media_categories,omitempty"`
	MediaDefault         string            `json:"media_default,omitempty"`
	TaskCooldownDuration string            `json:"task_cooldown_duration,omitempty"`
	TaskRole             string            `json:"task_role,omitempty"`
	TranslateChannels    []string          `json:"translate_channels,omitempty"`
}

func (g *GuildSettings) Key() string { return g.GuildID }

// CommandLogEntry is one recorded command invocation.
type CommandLogEntry struct {
	ID          uint64    `json:"id"`
	GuildID     string    `json:"guild_id"`
	ChannelID   string    `json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	GuildName   string    `json:"guild_name"`
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	Command     string    `json:"command"`
	Datetime    time.Time `json:"datetime"`
}

func (c *CommandLogEntry) Key() string { return guildRowKey(c.GuildID, c.ID) }

// PurgeJob.Mode values. These are written into every job row, so they are
// frozen the same way a key layout is: changing one orphans the jobs already on
// disk. Add a mode, never rename one.
const (
	PurgeModeDelayed   = "delayed"
	PurgeModeRecurring = "recurring"
)

// PurgeJob is a scheduled channel cleanup. One channel holds at most one job,
// which is what makes the channel id enough to address it.
type PurgeJob struct {
	GuildID    string    `json:"guild_id"`
	ChannelID  string    `json:"channel_id"`
	Mode       string    `json:"mode"`        // one of the PurgeMode constants
	DelayUntil time.Time `json:"delay_until"` // relevant only for PurgeModeDelayed
	OlderThan  string    `json:"older_than"`  // relevant only for PurgeModeRecurring
	StartedAt  time.Time `json:"started_at"`
	Silent     bool      `json:"silent"`
}

func (p *PurgeJob) Key() string { return guildScopedKey(p.GuildID, p.ChannelID) }

// ShortLink is one redirect record.
//
// It keys on the short id alone, not on guild plus short id: the redirect
// server resolves an incoming path with no guild in hand, so the id has to
// address the row on its own. That makes short ids global — see AddShortLink,
// which is where uniqueness is enforced.
type ShortLink struct {
	ShortID  string    `json:"short_id"`
	GuildID  string    `json:"guild_id"`
	Original string    `json:"original"`
	UserID   string    `json:"user_id"`
	Created  time.Time `json:"created"`
	Clicks   int       `json:"clicks"`
}

func (s *ShortLink) Key() string { return s.ShortID }

// Task.Status values. Written into every task row, and frozen for the same
// reason the key layouts are: a renamed status stops matching the rows already
// stored, and nothing reports an error — the task simply stops being found.
const (
	TaskStatusPending   = "pending"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
	TaskStatusSafeword  = "safeword"
)

// Task is a roleplay task currently held by one member.
type Task struct {
	GuildID    string    `json:"guild_id"`
	UserID     string    `json:"user_id"`
	MessageID  string    `json:"task_message_id"`
	AssignedAt time.Time `json:"assigned_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Status     string    `json:"status"` // one of the TaskStatus constants
}

func (t *Task) Key() string { return guildScopedKey(t.GuildID, t.UserID) }

// TaskCooldown blocks a member from drawing another task until Until passes.
type TaskCooldown struct {
	GuildID string    `json:"guild_id"`
	UserID  string    `json:"user_id"`
	Until   time.Time `json:"until"`
}

func (c *TaskCooldown) Key() string { return guildScopedKey(c.GuildID, c.UserID) }
