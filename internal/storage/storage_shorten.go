package storage

import (
	"fmt"
	"time"

	"github.com/keshon/datastore"
)

// AddShortLink stores a new redirect.
//
// Short ids are global, not per-guild: the redirect server resolves an incoming
// path with no guild in hand. A collision would silently repoint someone else's
// link, so this refuses one rather than overwriting.
func (s *Storage) AddShortLink(guildID, userID, original, shortID string) error {
	if _, exists := s.shortLink.Get(shortID); exists {
		return fmt.Errorf("short link with ID '%s' already exists", shortID)
	}
	return s.shortLink.Put(&ShortLink{
		ShortID:  shortID,
		GuildID:  guildID,
		Original: original,
		UserID:   userID,
		Clicks:   0,
		Created:  time.Now(),
	})
}

// GetUserShortLinks lists one member's links in a guild.
func (s *Storage) GetUserShortLinks(guildID, userID string) ([]ShortLink, error) {
	var links []ShortLink
	for _, l := range s.shortLinkByGuild.Find(guildID) {
		if l.UserID == userID {
			links = append(links, *l)
		}
	}
	return links, nil
}

// ClearUserShortLinks deletes all short links belonging to a specific user.
func (s *Storage) ClearUserShortLinks(guildID, userID string) error {
	return s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.shortLink)
		for _, l := range datastore.InIndex(tx, s.shortLinkByGuild).Find(guildID) {
			if l.UserID != userID {
				continue
			}
			if err := col.Delete(l.Key()); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteShortLink removes a single short link by its shortID for the specified user.
func (s *Storage) DeleteShortLink(guildID, userID, shortID string) error {
	link, ok := s.shortLink.Get(shortID)
	// Match on owner and guild as well as id: the id alone addresses every
	// link in the store, and a bare lookup would let one member delete another's.
	if !ok || link.UserID != userID || link.GuildID != guildID {
		return fmt.Errorf("short link with ID '%s' not found", shortID)
	}
	return s.shortLink.Delete(shortID)
}

// IncrementClicks increments the click count for a specific short link.
//
// The read and the write share a transaction because the redirect server
// handles requests concurrently, and a plain read-modify-write would lose
// counts whenever two hits on the same link overlap.
func (s *Storage) IncrementClicks(guildID, shortID string) error {
	return s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.shortLink)
		link, ok := col.Get(shortID)
		if !ok || link.GuildID != guildID {
			return fmt.Errorf("short link with ID '%s' not found", shortID)
		}
		link.Clicks++
		return col.Put(link)
	})
}

// FindLinkByID resolves a short id to its guild and link.
func (s *Storage) FindLinkByID(shortID string) (string, *ShortLink, error) {
	link, ok := s.shortLink.Get(shortID)
	if !ok {
		return "", nil, fmt.Errorf("short link with ID '%s' not found", shortID)
	}
	return link.GuildID, link, nil
}
