package storage

import (
	"fmt"
	"slices"
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
