package storage

import (
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

// IrritateMindPerson raises how much someone has got on her nerves.
//
// Takes the already-decayed current level from the caller rather than decaying
// here, because the decay needs the same clock the prompt was built with and
// storage has no business knowing the half-life.
func (s *Storage) IrritateMindPerson(guildID, userID string, level float64, at time.Time) error {
	if guildID == "" || userID == "" {
		return fmt.Errorf("storage: irritation needs a guild and a user")
	}

	return s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.mindPeople)

		person, ok := col.Get(guildScopedKey(guildID, userID))
		if !ok {
			person = &MindPerson{GuildID: guildID, UserID: userID, FirstSeen: at}
		}
		person.Irritation = level
		person.IrritatedAt = at
		return col.Put(person)
	})
}

// ForgetMindMemories deletes everything she remembers about a guild, and
// clears what she holds against the people in it. It reports how many memories
// went.
//
// Irritation goes with the memories deliberately. Clearing one without the
// other leaves her short with someone for a reason she can no longer name,
// which is the exact failure the irritation memory was added to prevent — a
// feeling with no cause attached.
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

		people := datastore.In(tx, s.mindPeople)
		for _, p := range datastore.InIndex(tx, s.mindPeopleByGuild).Find(guildID) {
			if p.Irritation == 0 && p.IrritatedAt.IsZero() {
				continue
			}
			p.Irritation = 0
			p.IrritatedAt = time.Time{}
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
