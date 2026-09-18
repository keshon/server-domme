package storage

import (
	"fmt"
	"time"

	"github.com/keshon/datastore"
)

// mindJournalLimit is how many decisions a channel keeps. Enough to look back
// over a conversation that went wrong, not a history of the server: the
// entries carry excerpts of what people said, and a record of everything is
// the wrong thing to keep for a debugging aid.
const mindJournalLimit = 50

// MindJournal is one decision she made about one message, and what came of
// it: why she answered or did not, what she was told, what the model gave
// back, what was posted and how it went down. It exists to answer "why did she
// do that?" about a specific moment — see /chat why.
type MindJournal struct {
	ID        uint64    `json:"id"`
	GuildID   string    `json:"guild_id"`
	ChannelID string    `json:"channel_id"`
	At        time.Time `json:"at"`

	// MessageID is the message she was deciding about, UserID and Username
	// who sent it, and Excerpt the start of what it said.
	MessageID string `json:"message_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	Username  string `json:"username,omitempty"`
	Excerpt   string `json:"excerpt,omitempty"`

	// Trigger is how the message reached her, Closer whether it closed the
	// topic, and Rule, Chance and Roll how the decision was reached.
	Trigger string  `json:"trigger"`
	Closer  bool    `json:"closer,omitempty"`
	Rule    string  `json:"rule,omitempty"`
	Chance  float64 `json:"chance,omitempty"`
	Roll    float64 `json:"roll,omitempty"`

	// Mood and Attitude are her state at the time, in words.
	Mood     string `json:"mood,omitempty"`
	Attitude string `json:"attitude,omitempty"`

	// Outcome is where it ended — answered, silent, declined, dropped, held —
	// and Reason why, in a phrase.
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`

	// Told are the instructions about her state the reply carried, Thought
	// her private line, Raw what the model returned and Posted what went
	// out, both trimmed.
	Told    []string `json:"told,omitempty"`
	Thought string   `json:"thought,omitempty"`
	// Perceived is how the model labelled their message when asked, or
	// "unreadable" when it was asked and gave no usable word. Shadow only:
	// nothing acts on it yet. See mind.Perception.
	Perceived string `json:"perceived,omitempty"`
	// Payoff is how something she started was received, and what that did
	// to her, once it is known. See mind.Payoff.
	Payoff string `json:"payoff,omitempty"`
	Raw    string `json:"raw,omitempty"`
	Posted string `json:"posted,omitempty"`

	// Backend is which relay answered and Took how long the reply took.
	Backend string        `json:"backend,omitempty"`
	Took    time.Duration `json:"took,omitempty"`

	// ReplyID is her message, and Reaction how it landed, when it did.
	ReplyID  string `json:"reply_id,omitempty"`
	Reaction string `json:"reaction,omitempty"`
}

func (j *MindJournal) Key() string { return guildRowKey(j.GuildID, j.ID) }

func journalChannel(guildID, channelID string) string { return guildID + ":" + channelID }

// AddMindJournal records a decision and returns its id, trimming the
// channel's oldest.
func (s *Storage) AddMindJournal(j MindJournal) (uint64, error) {
	if j.GuildID == "" || j.ChannelID == "" {
		return 0, fmt.Errorf("storage: journal entry needs a guild and a channel")
	}
	if j.At.IsZero() {
		j.At = time.Now()
	}

	entry := &j
	err := s.db.Update(func(tx *datastore.Tx) error {
		entry.ID = tx.NextID("mindjournal:" + entry.GuildID)
		col := datastore.In(tx, s.mindJournal)
		if err := col.Put(entry); err != nil {
			return err
		}
		existing := datastore.InIndex(tx, s.mindJournalByChannel).Find(journalChannel(entry.GuildID, entry.ChannelID))
		return trimOldest(col, existing, mindJournalLimit)
	})
	if err != nil {
		return 0, fmt.Errorf("storage: add journal: %w", err)
	}
	return entry.ID, nil
}

// UpdateMindJournal applies change to one entry. A missing entry — trimmed,
// or forgotten — is not an error: the journal is a record, and a record that
// has moved on has nothing to update.
func (s *Storage) UpdateMindJournal(guildID string, id uint64, change func(*MindJournal)) error {
	if guildID == "" || id == 0 {
		return nil
	}
	err := s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.mindJournal)
		entry, ok := col.Get(guildRowKey(guildID, id))
		if !ok {
			return nil
		}
		change(entry)
		return col.Put(entry)
	})
	if err != nil {
		return fmt.Errorf("storage: update journal: %w", err)
	}
	return nil
}

// MindJournalIn lists a channel's decisions, oldest first.
func (s *Storage) MindJournalIn(guildID, channelID string) []MindJournal {
	rows := s.mindJournalByChannel.Find(journalChannel(guildID, channelID))
	out := make([]MindJournal, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	return out
}

// MindDay counts what she did in a guild on one day, in the community's
// timezone: how often she answered, stayed quiet, declined, caught herself
// repeating, was liked or panned, and how often no relay answered. Counts
// rather than the journal because a busy channel turns its fifty entries over
// in an hour, and whether a change made her better is a question about a day.
type MindDay struct {
	GuildID string         `json:"guild_id"`
	Day     string         `json:"day"`
	Counts  map[string]int `json:"counts"`
}

func (d *MindDay) Key() string { return guildScopedKey(d.GuildID, d.Day) }

// CountMindEvent adds one to a named count for a guild's day.
func (s *Storage) CountMindEvent(guildID, day, event string) error {
	if guildID == "" || day == "" || event == "" {
		return nil
	}
	err := s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.mindDays)
		d, ok := col.Get(guildScopedKey(guildID, day))
		if !ok {
			d = &MindDay{GuildID: guildID, Day: day}
		}
		if d.Counts == nil {
			d.Counts = make(map[string]int)
		}
		d.Counts[event]++
		return col.Put(d)
	})
	if err != nil {
		return fmt.Errorf("storage: count event: %w", err)
	}
	return nil
}

// MindDayCounts returns a guild's counts for a day, never nil.
func (s *Storage) MindDayCounts(guildID, day string) map[string]int {
	d, ok := s.mindDays.Get(guildScopedKey(guildID, day))
	if !ok || d.Counts == nil {
		return map[string]int{}
	}
	return d.Counts
}
