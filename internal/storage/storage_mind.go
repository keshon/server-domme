package storage

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/keshon/datastore"
)

// SeeMindPerson records that a member spoke, returning the updated row.
//
// The read and the write share a transaction because one member can post in
// two channels at once and both handlers land here; a plain read-modify-write
// would lose messages from the count, silently and with nothing to report.
func (s *Storage) SeeMindPerson(guildID, userID, username string, at time.Time) (*MindPerson, error) {
	if guildID == "" || userID == "" {
		return nil, fmt.Errorf("storage: mind person needs a guild and a user")
	}

	var updated *MindPerson
	err := s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.mindPeople)

		person, ok := col.Get(guildScopedKey(guildID, userID))
		if !ok {
			person = &MindPerson{GuildID: guildID, UserID: userID, FirstSeen: at}
		}

		person.Messages++
		person.PrevSeen = person.LastSeen
		person.LastSeen = at
		if username != "" {
			person.Username = username
		}
		if person.FirstSeen.IsZero() {
			person.FirstSeen = at
		}

		updated = person
		return col.Put(person)
	})
	if err != nil {
		return nil, fmt.Errorf("storage: record mind person: %w", err)
	}
	return updated, nil
}

// GetMindPerson returns what is known about a member, or nil when the bot has
// not seen them speak. A missing row is not an error: it is the ordinary state
// for everyone who has not said anything yet.
func (s *Storage) GetMindPerson(guildID, userID string) *MindPerson {
	person, ok := s.mindPeople.Get(guildScopedKey(guildID, userID))
	if !ok {
		return nil
	}
	return person
}

// AddChatChannel opts a channel into the chat persona.
//
// Chat is opt-in per channel rather than per guild, and deliberately so: the
// bot reads the channel and sends its contents to a third-party relay to get a
// reply, which is a different privacy posture from the rest of this bot. An
// admin naming the channel is the consent for that.
func (s *Storage) AddChatChannel(guildID, channelID string) error {
	g := s.guildSettings(guildID)
	if slices.Contains(g.ChatChannels, channelID) {
		return fmt.Errorf("storage: channel already in chat list")
	}
	g.ChatChannels = append(g.ChatChannels, channelID)
	return s.settings.Put(g)
}

// RemoveChatChannel opts a channel back out.
func (s *Storage) RemoveChatChannel(guildID, channelID string) error {
	g := s.guildSettings(guildID)
	if len(g.ChatChannels) == 0 {
		return fmt.Errorf("storage: no chat channels configured")
	}
	updated := slices.DeleteFunc(g.ChatChannels, func(c string) bool {
		return c == channelID
	})
	if len(updated) == len(g.ChatChannels) {
		return fmt.Errorf("storage: channel not found in chat list")
	}
	g.ChatChannels = updated
	// Volunteering only makes sense on top of answering, so taking a channel
	// away takes that with it. Left behind, it would come back switched on the
	// next time someone ran /chat here, which nobody asking for it back would
	// expect.
	g.ChatProactive = slices.DeleteFunc(g.ChatProactive, func(c string) bool {
		return c == channelID
	})
	return s.settings.Put(g)
}

// GetChatChannels lists the guild's chat channels, never nil.
func (s *Storage) GetChatChannels(guildID string) []string {
	channels := s.guildSettings(guildID).ChatChannels
	if channels == nil {
		return []string{}
	}
	return channels
}

// AllChatChannels lists every opted-in channel across all guilds, as a map
// from channel to guild. Used once at startup, to reread the conversations a
// restart interrupted; see chat.Service.catchUp.
func (s *Storage) AllChatChannels() map[string]string {
	out := make(map[string]string)
	for g := range s.settings.All() {
		for _, channelID := range g.ChatChannels {
			out[channelID] = g.GuildID
		}
	}
	return out
}

// IsChatChannel reports whether a channel is opted in.
func (s *Storage) IsChatChannel(guildID, channelID string) bool {
	return slices.Contains(s.guildSettings(guildID).ChatChannels, channelID)
}

// SetChatBrief stores the guild's description of itself, which is the one
// piece of grounding nothing can derive from the gateway.
func (s *Storage) SetChatBrief(guildID, brief string) error {
	g := s.guildSettings(guildID)
	g.ChatBrief = brief
	return s.settings.Put(g)
}

// GetChatBrief returns the guild's self-description, empty when unset.
func (s *Storage) GetChatBrief(guildID string) string {
	return s.guildSettings(guildID).ChatBrief
}

// MarkMindSpoke records that the character spoke in a guild.
//
// Stored rather than derived because the conversation buffer keeps thirty
// minutes and this has to answer "how long has she been alone", which is a
// question measured in hours. See mind.DeriveDrives.
func (s *Storage) MarkMindSpoke(guildID string, at time.Time) error {
	if guildID == "" {
		return nil
	}
	err := s.db.Update(func(tx *datastore.Tx) error {
		return datastore.In(tx, s.mindGuilds).Put(&MindGuild{
			GuildID:     guildID,
			LastSpokeAt: at,
		})
	})
	if err != nil {
		return fmt.Errorf("storage: mark mind spoke: %w", err)
	}
	return nil
}

// GetMindGuild returns the character's state in a guild, or nil when she has
// never spoken there.
func (s *Storage) GetMindGuild(guildID string) *MindGuild {
	got, ok := s.mindGuilds.Get(guildID)
	if !ok {
		return nil
	}
	return got
}

// mindMemoryLimit is how many memories a guild keeps. Generous, because a row
// is two sentences and the read path already drops anything too dim to be
// worth rendering — this only stops the collection growing without bound over
// years.
const mindMemoryLimit = 500

// AddMindMemory records something worth remembering.
//
// Append-and-trim in one transaction, the same shape as SetCommand: the index
// is read inside the transaction so the rows trimmed against are the rows the
// commit sees.
func (s *Storage) AddMindMemory(m MindMemory) error {
	if m.GuildID == "" || strings.TrimSpace(m.Gist) == "" {
		return fmt.Errorf("storage: mind memory needs a guild and a gist")
	}
	if m.At.IsZero() {
		m.At = time.Now()
	}

	entry := &m
	return s.db.Update(func(tx *datastore.Tx) error {
		entry.ID = tx.NextID("mindmem:" + entry.GuildID)
		col := datastore.In(tx, s.mindMemories)
		if err := col.Put(entry); err != nil {
			return err
		}
		existing := datastore.InIndex(tx, s.mindMemoriesByGuild).Find(entry.GuildID)
		return trimOldest(col, existing, mindMemoryLimit)
	})
}

// MindMemories returns a channel's memories, oldest first.
//
// Filtered by channel in Go rather than by a second index: a guild holds at
// most mindMemoryLimit rows, and an index per channel would cost a write on
// every message to save a scan of a few hundred records on a read that only
// happens when she is about to speak.
func (s *Storage) MindMemories(guildID, channelID string) []MindMemory {
	rows := s.mindMemoriesByGuild.Find(guildID)
	out := make([]MindMemory, 0, len(rows))
	for _, r := range rows {
		if channelID != "" && r.ChannelID != channelID {
			continue
		}
		out = append(out, *r)
	}
	return out
}

// NoteMindPerson replaces what she knows about someone: their facts, already
// merged by the caller, and her impression when impression is not empty.
//
// Whole-list replacement rather than a merge here, because merging is a
// decision about which value wins and that belongs with the rest of the
// cognition in mind.MergeFacts, not in storage.
func (s *Storage) NoteMindPerson(guildID, userID string, facts []MindFact, impression string, at time.Time) error {
	if guildID == "" || userID == "" {
		return fmt.Errorf("storage: notes need a guild and a user")
	}

	return s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.mindPeople)

		person, ok := col.Get(guildScopedKey(guildID, userID))
		if !ok {
			person = &MindPerson{GuildID: guildID, UserID: userID, FirstSeen: at}
		}
		person.Facts = facts
		if impression != "" {
			person.Impression = impression
			person.ImpressionAt = at
		}
		return col.Put(person)
	})
}

// ForgetMindMemories deletes everything she remembers about a guild, and
// clears what she holds for and against the people in it: irritation, warmth,
// facts and impressions. It reports how many memories went.
//
// Feelings go with the memories deliberately. Clearing one without the other
// leaves her short with someone, or fond of them, for a reason she can no
// longer name — a feeling with no cause attached, which is the exact failure
// the irritation memory was added to prevent.
//
// Message counts and first-seen stamps survive. Those are how she knows a
// regular from a stranger, and wiping them turns everyone in the server into a
// newcomer, which is a much larger thing than an administrator asking her to
// forget what happened.
func (s *Storage) ForgetMindMemories(guildID string) (int, error) {
	if guildID == "" {
		return 0, fmt.Errorf("storage: forget memories needs a guild")
	}

	var forgotten int
	err := s.db.Update(func(tx *datastore.Tx) error {
		memories := datastore.In(tx, s.mindMemories)
		for _, m := range datastore.InIndex(tx, s.mindMemoriesByGuild).Find(guildID) {
			if err := memories.Delete(m.Key()); err != nil {
				return err
			}
			forgotten++
		}

		// The journal goes too: it carries excerpts of what people said and
		// what she thought of them, which is exactly what was asked to go.
		journal := datastore.In(tx, s.mindJournal)
		for _, j := range datastore.InIndex(tx, s.mindJournalByGuild).Find(guildID) {
			if err := journal.Delete(j.Key()); err != nil {
				return err
			}
		}

		people := datastore.In(tx, s.mindPeople)
		for _, p := range datastore.InIndex(tx, s.mindPeopleByGuild).Find(guildID) {
			if p.Tension == 0 && p.TensionAt.IsZero() && p.Closeness == 0 &&
				p.WelcomeAt.IsZero() && p.LastEvent == "" &&
				len(p.Facts) == 0 && p.Impression == "" {
				continue
			}
			p.Tension, p.TensionAt = 0, time.Time{}
			p.Closeness, p.ClosenessAt = 0, time.Time{}
			p.Welcome, p.WelcomeAt = 0, time.Time{}
			p.LastEvent, p.LastEventAt = "", time.Time{}
			p.Facts = nil
			p.Impression, p.ImpressionAt = "", time.Time{}
			if err := people.Put(p); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("storage: forget memories: %w", err)
	}
	return forgotten, nil
}

// SetChatRoleBias records what a role means to the persona. A zero regard with
// no note removes the entry rather than storing a row that says nothing.
func (s *Storage) SetChatRoleBias(guildID, roleID string, bias ChatRoleBias) error {
	if guildID == "" || roleID == "" {
		return fmt.Errorf("storage: role bias needs a guild and a role")
	}

	return s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.settings)

		settings, ok := col.Get(guildID)
		if !ok {
			settings = &GuildSettings{GuildID: guildID}
		}
		if settings.ChatRoles == nil {
			settings.ChatRoles = make(map[string]ChatRoleBias)
		}

		if bias.Regard == 0 && strings.TrimSpace(bias.Note) == "" {
			delete(settings.ChatRoles, roleID)
		} else {
			settings.ChatRoles[roleID] = bias
		}
		return col.Put(settings)
	})
}

// ChatRoleBiases returns how she regards each role in a guild.
func (s *Storage) ChatRoleBiases(guildID string) map[string]ChatRoleBias {
	settings, ok := s.settings.Get(guildID)
	if !ok || settings.ChatRoles == nil {
		return nil
	}

	out := make(map[string]ChatRoleBias, len(settings.ChatRoles))
	for id, bias := range settings.ChatRoles {
		out[id] = bias
	}
	return out
}

// ErrChatChannelRequired is returned when proactivity is asked for in a
// channel she has not been let into. Its text is shown to the administrator,
// which is why it carries no package prefix.
var ErrChatChannelRequired = errors.New("she has to be let into this channel with /chat here first")

// SetChatProactive lets her speak unprompted in a channel, or stops her.
//
// Refuses a channel she cannot answer in: volunteering is a permission on top
// of answering, not instead of it.
func (s *Storage) SetChatProactive(guildID, channelID string, on bool) error {
	g := s.guildSettings(guildID)
	if on && !slices.Contains(g.ChatChannels, channelID) {
		return ErrChatChannelRequired
	}

	g.ChatProactive = slices.DeleteFunc(g.ChatProactive, func(c string) bool {
		return c == channelID
	})
	if on {
		g.ChatProactive = append(g.ChatProactive, channelID)
	}
	return s.settings.Put(g)
}

// IsChatProactive reports whether she may speak unprompted in a channel.
//
// Checks both lists rather than trusting the subset to hold, so a row edited
// by hand or restored from an old backup cannot have her volunteering in a
// channel she is not allowed to read.
func (s *Storage) IsChatProactive(guildID, channelID string) bool {
	g := s.guildSettings(guildID)
	return slices.Contains(g.ChatChannels, channelID) &&
		slices.Contains(g.ChatProactive, channelID)
}

// MindChannelState returns what she has volunteered in a channel, or a zero
// row when she never has.
func (s *Storage) MindChannelState(guildID, channelID string) MindChannel {
	got, ok := s.mindChannels.Get(guildScopedKey(guildID, channelID))
	if !ok {
		return MindChannel{GuildID: guildID, ChannelID: channelID}
	}
	return *got
}

// MarkVolunteered records one unprompted remark, restarting the count when
// day has moved on.
//
// The day is the caller's to compute, because it has to be in the
// community's timezone and storage has no business knowing which that is.
func (s *Storage) MarkVolunteered(guildID, channelID, day string, at time.Time) error {
	if guildID == "" || channelID == "" {
		return fmt.Errorf("storage: volunteer needs a guild and a channel")
	}

	return s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.mindChannels)

		row, ok := col.Get(guildScopedKey(guildID, channelID))
		if !ok {
			row = &MindChannel{GuildID: guildID, ChannelID: channelID}
		}
		if row.Day != day {
			row.Day, row.Today = day, 0
		}
		row.Today++
		row.VolunteeredAt = at
		return col.Put(row)
	})
}
