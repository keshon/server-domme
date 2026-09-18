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
	ChatChannels         []string          `json:"chat_channels,omitempty"`
	ChatBrief            string            `json:"chat_brief,omitempty"`
	// ChatProactive lists the chat channels where she may also speak without
	// being asked. Always a subset of ChatChannels: answering and volunteering
	// are separate permissions, and the second only makes sense on top of the
	// first.
	ChatProactive []string `json:"chat_proactive,omitempty"`
	// WelcomeGifs are the links /welcome picks from at random, shared by
	// every role.
	WelcomeGifs []string `json:"welcome_gifs,omitempty"`
	// ChatAttentionOff stops her reaching out to anyone in the guild, whatever
	// they opted into. An administrator's override of members' consent.
	ChatAttentionOff bool `json:"chat_attention_off,omitempty"`
	// ChatRoles is how she regards each Discord role, keyed by role id.
	ChatRoles map[string]ChatRoleBias `json:"chat_roles,omitempty"`
}

// ChatRoleBias is what a Discord role means to the persona.
//
// Per role rather than per person because a server with roles has already
// decided who is what, and rating members one at a time is asking an operator
// not to use it.
type ChatRoleBias struct {
	// Regard runs -1 to +1. Zero says nothing.
	Regard float64 `json:"regard"`
	// Note is an instruction about anyone holding this role, used verbatim in
	// the prompt. "a submissive here, speak to them as one" says something no
	// number can, which is why it is here at all.
	Note string `json:"note,omitempty"`
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

// MindPerson is what the chat persona has observed about one member of one
// guild.
//
// It holds only things the bot counted itself — how many messages it has seen
// from them and when — with nothing a language model inferred. That is what
// lets this row be trusted: a summary written by a relay we do not control
// would be an unverifiable claim about a real person, stored under their id.
type MindPerson struct {
	GuildID   string    `json:"guild_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Messages  int       `json:"messages"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	// PrevSeen is the LastSeen value this sighting replaced, which is the only
	// way to tell an absence from an ongoing conversation: after LastSeen is
	// stamped with now, the gap it used to describe is gone. Whether a gap is
	// long enough to be worth remarking on is not decided here — see
	// mind.Acquaintance.
	PrevSeen time.Time `json:"prev_seen,omitempty"`
	// Tension is how much this person has got on her nerves, 0..1, as it
	// stood at IrritatedAt. It is stored undecayed and decayed on read, so
	// there is nothing to sweep and a restart loses nothing — see
	// mind.IrritationNow.
	//
	// Per person rather than per guild on purpose: being short with one member
	// and perfectly ordinary with the next is the thing that separates someone
	// annoyed from a bot in a bad mode.
	Tension   float64   `json:"irritation,omitempty"`
	TensionAt time.Time `json:"irritated_at,omitempty"`
	// Closeness is how much she has come to like them, 0..1, as it stood at
	// WarmAt. Stored and decayed the same way as Irritation, only slower.
	Closeness   float64   `json:"warmth,omitempty"`
	ClosenessAt time.Time `json:"warm_at,omitempty"`
	// Welcome is how they have taken it when she came to them, as it stood
	// at WelcomeAt; unset reads as neutral. See mind.Bond.
	Welcome   float64   `json:"welcome,omitempty"`
	WelcomeAt time.Time `json:"welcome_at,omitempty"`
	// LastEvent is the last thing that moved how she feels about them, and
	// LastEventAt when — so "why is she like this with me" has an answer.
	LastEvent   string    `json:"last_event,omitempty"`
	LastEventAt time.Time `json:"last_event_at,omitempty"`
	// Facts are what they have said about themselves, newest first, and
	// Impression is her one-line opinion of them. Both are written after a
	// conversation is remembered; see mind.NotesPrompt.
	Facts        []MindFact `json:"facts,omitempty"`
	Impression   string     `json:"impression,omitempty"`
	ImpressionAt time.Time  `json:"impression_at,omitempty"`
	// Concerns are things they said they were about to do, which she may
	// have on her mind. See mind.Concern.
	Concerns []MindConcern `json:"concerns,omitempty"`

	// Attention is how much reaching out they have agreed to — light, keen,
	// insistent — or empty for none, which is everyone who has not asked.
	// See mind.TriggerReach.
	Attention string `json:"attention,omitempty"`
	// LastExchangeAt is when they last spoke to her, and LastChatChannel
	// where; LastActiveAt when they were last seen anywhere in the server,
	// kept only for people who opted in.
	LastExchangeAt  time.Time `json:"last_exchange_at,omitempty"`
	LastChatChannel string    `json:"last_chat_channel,omitempty"`
	LastActiveAt    time.Time `json:"last_active_at,omitempty"`
	// ReachedAt is when she last reached out to them, ReachDay and
	// ReachToday how many times on that day, and Unanswered how many in a
	// row they have not answered.
	ReachedAt  time.Time `json:"reached_at,omitempty"`
	ReachDay   string    `json:"reach_day,omitempty"`
	ReachToday int       `json:"reach_today,omitempty"`
	Unanswered int       `json:"unanswered,omitempty"`
}

// MindConcern is something a person said they were about to do; see
// mind.Concern.
type MindConcern struct {
	What    string    `json:"what"`
	Due     time.Time `json:"due"`
	Expires time.Time `json:"expires"`
	Noted   time.Time `json:"noted"`
	Passed  int       `json:"passed,omitempty"`
}

// MindFact is one thing a person said about themselves.
type MindFact struct {
	Key   string    `json:"key"`
	Value string    `json:"value"`
	At    time.Time `json:"at"`
}

func (m *MindPerson) Key() string { return guildScopedKey(m.GuildID, m.UserID) }

// MindGuild is the character's own state in one guild, as opposed to what it
// knows about the people in it.
//
// One row per guild, holding only what cannot be recomputed. The live
// conversation comes back from Discord on demand (see chat.Service.backfill)
// and the drives are derived on read from these timestamps, so nothing here is
// a second copy of something Discord already stores.
type MindGuild struct {
	GuildID string `json:"guild_id"`
	// LastSpokeAt is when she last said something here. It is what separates
	// a quiet hour from a quiet week, which the conversation buffer cannot:
	// that only keeps thirty minutes.
	LastSpokeAt time.Time `json:"last_spoke_at,omitempty"`
	// LastContactAt is when someone last spoke to her here. It, not her own
	// speaking, is what satisfies her need for company: talking into a room
	// that does not answer cures no loneliness. See mind.MoodInput.
	LastContactAt time.Time `json:"last_contact_at,omitempty"`
	// Initiative is how much she has put herself forward lately, as of
	// InitiativeAt; decayed on read. See mind.Fatigue.
	Initiative   float64   `json:"initiative,omitempty"`
	InitiativeAt time.Time `json:"initiative_at,omitempty"`
	// MoodSwing is how far events have moved her mood from its baseline, as
	// of MoodSwingAt; decayed on read. See mind.MoodSwing.
	MoodSwing   float64   `json:"mood_swing,omitempty"`
	MoodSwingAt time.Time `json:"mood_swing_at,omitempty"`
}

func (m *MindGuild) Key() string { return m.GuildID }

// MindChannel is what she has volunteered in one channel, kept so the daily
// budget survives a restart. Held in memory it would reset on every deploy,
// and a bot redeployed three times in an afternoon would get three budgets.
type MindChannel struct {
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
	// VolunteeredAt is when she last spoke here unprompted.
	VolunteeredAt time.Time `json:"volunteered_at,omitempty"`
	// Day is the calendar day, in the community's timezone, that Today
	// counts. A different day means Today is stale and starts again.
	Day   string `json:"day,omitempty"`
	Today int    `json:"today,omitempty"`
}

func (m *MindChannel) Key() string { return guildScopedKey(m.GuildID, m.ChannelID) }

// MindMemory is one thing the character remembers happening in a channel.
//
// Append-only, so the key zero-pads its id and lexicographic order is
// chronological — the same shape as the command log, and for the same reason.
// Records are never rewritten: what fades is how much of one is rendered, not
// what is stored. See mind.Memory.
type MindMemory struct {
	ID        uint64    `json:"id"`
	GuildID   string    `json:"guild_id"`
	ChannelID string    `json:"channel_id"`
	At        time.Time `json:"at"`
	Gist      string    `json:"gist"`
	Detail    string    `json:"detail,omitempty"`
	// Weight is how much the moment mattered, 0..1. It slows the memory's
	// decay rather than raising its brightness.
	Weight float64 `json:"weight,omitempty"`
	// People are the user ids who were there, which is what lets a memory
	// return because of who is in the room rather than what is being said.
	People []string `json:"people,omitempty"`
}

func (m *MindMemory) Key() string { return guildRowKey(m.GuildID, m.ID) }

// TaskCooldown blocks a member from drawing another task until Until passes.
type TaskCooldown struct {
	GuildID string    `json:"guild_id"`
	UserID  string    `json:"user_id"`
	Until   time.Time `json:"until"`
}

func (c *TaskCooldown) Key() string { return guildScopedKey(c.GuildID, c.UserID) }
