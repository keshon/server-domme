package storage

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/keshon/datastore"
)

// WelcomeRole is what welcoming someone with one role means on one server:
// where their introduction goes and what it says, and where their welcome
// goes and what that says. Per role because a server that has roles has
// already decided people are welcomed differently — a sub and a Domme are
// pointed at different channels and told different things.
type WelcomeRole struct {
	GuildID string `json:"guild_id"`
	RoleID  string `json:"role_id"`

	IntroChannel  string `json:"intro_channel,omitempty"`
	IntroTemplate string `json:"intro_template,omitempty"`

	WelcomeChannel  string `json:"welcome_channel,omitempty"`
	WelcomeTemplate string `json:"welcome_template,omitempty"`
}

func (w *WelcomeRole) Key() string { return guildScopedKey(w.GuildID, w.RoleID) }

// Welcomed records that someone was welcomed for a role, part by part, so a
// second run cannot post the same thing twice by accident and a run that
// failed half way can finish the other half.
type Welcomed struct {
	GuildID string    `json:"guild_id"`
	UserID  string    `json:"user_id"`
	RoleID  string    `json:"role_id"`
	IntroAt time.Time `json:"intro_at,omitempty"`
	// WelcomeAt is when the welcome went out, and By who sent it.
	WelcomeAt time.Time `json:"welcome_at,omitempty"`
	By        string    `json:"by,omitempty"`
}

func (w *Welcomed) Key() string { return guildScopedKey(w.GuildID, w.UserID+":"+w.RoleID) }

// maxWelcomeGifs bounds the gif list. A pool to pick from, not a gallery.
// ErrWelcomeGifsFull states the number; change both together.
const maxWelcomeGifs = 50

// WelcomeRoleFor returns a role's welcome settings, or nil when none are set.
func (s *Storage) WelcomeRoleFor(guildID, roleID string) *WelcomeRole {
	w, ok := s.welcomeRoles.Get(guildScopedKey(guildID, roleID))
	if !ok {
		return nil
	}
	copied := *w
	return &copied
}

// WelcomeRoles lists every role with welcome settings in a guild.
func (s *Storage) WelcomeRoles(guildID string) []WelcomeRole {
	rows := s.welcomeRolesByGuild.Find(guildID)
	out := make([]WelcomeRole, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	return out
}

// UpdateWelcomeRole applies change to a role's settings, creating them when
// there are none.
func (s *Storage) UpdateWelcomeRole(guildID, roleID string, change func(*WelcomeRole)) error {
	if guildID == "" || roleID == "" {
		return fmt.Errorf("storage: welcome settings need a guild and a role")
	}
	err := s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.welcomeRoles)
		w, ok := col.Get(guildScopedKey(guildID, roleID))
		if !ok {
			w = &WelcomeRole{GuildID: guildID, RoleID: roleID}
		}
		change(w)
		return col.Put(w)
	})
	if err != nil {
		return fmt.Errorf("storage: update welcome role: %w", err)
	}
	return nil
}

// RemoveWelcomeRole deletes a role's welcome settings. Records of who was
// welcomed stay: they are what stops a re-added role welcoming everyone again.
func (s *Storage) RemoveWelcomeRole(guildID, roleID string) error {
	err := s.db.Update(func(tx *datastore.Tx) error {
		return datastore.In(tx, s.welcomeRoles).Delete(guildScopedKey(guildID, roleID))
	})
	if err != nil {
		return fmt.Errorf("storage: remove welcome role: %w", err)
	}
	return nil
}

// ErrWelcomeRoleMissing and ErrWelcomeRoleTaken are why a move is refused.
var (
	ErrWelcomeRoleMissing = errors.New("storage: the role has no welcome to move")
	ErrWelcomeRoleTaken   = errors.New("storage: the role already has a welcome")
)

// MoveWelcomeRole gives one role's welcome settings to another, then lets
// change adjust them, in one step. keep leaves the first role's settings as
// they were, making it a copy. A role that already has settings is not
// written over: what it had might be the real thing, and a move is for
// carrying a finished draft over to an empty role. Who was welcomed stays
// with the role they were welcomed as.
func (s *Storage) MoveWelcomeRole(guildID, fromRoleID, toRoleID string, keep bool, change func(*WelcomeRole)) error {
	if guildID == "" || fromRoleID == "" || toRoleID == "" {
		return fmt.Errorf("storage: moving a welcome needs a guild and two roles")
	}
	return s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.welcomeRoles)
		from, ok := col.Get(guildScopedKey(guildID, fromRoleID))
		if !ok {
			return ErrWelcomeRoleMissing
		}
		if _, taken := col.Get(guildScopedKey(guildID, toRoleID)); taken {
			return ErrWelcomeRoleTaken
		}
		to := *from
		to.RoleID = toRoleID
		if change != nil {
			change(&to)
		}
		if err := col.Put(&to); err != nil {
			return err
		}
		if keep {
			return nil
		}
		return col.Delete(guildScopedKey(guildID, fromRoleID))
	})
}

// WelcomedFor returns the record of someone's welcome for a role, or nil.
func (s *Storage) WelcomedFor(guildID, userID, roleID string) *Welcomed {
	w, ok := s.welcomed.Get(guildScopedKey(guildID, userID+":"+roleID))
	if !ok {
		return nil
	}
	copied := *w
	return &copied
}

// MarkWelcomed records the parts of a welcome that went out.
func (s *Storage) MarkWelcomed(guildID, userID, roleID, by string, intro, welcome bool, at time.Time) error {
	err := s.db.Update(func(tx *datastore.Tx) error {
		col := datastore.In(tx, s.welcomed)
		w, ok := col.Get(guildScopedKey(guildID, userID+":"+roleID))
		if !ok {
			w = &Welcomed{GuildID: guildID, UserID: userID, RoleID: roleID}
		}
		if intro {
			w.IntroAt = at
		}
		if welcome {
			w.WelcomeAt = at
		}
		w.By = by
		return col.Put(w)
	})
	if err != nil {
		return fmt.Errorf("storage: mark welcomed: %w", err)
	}
	return nil
}

// WelcomeGifs lists the guild's welcome gifs.
func (s *Storage) WelcomeGifs(guildID string) []string {
	return slices.Clone(s.guildSettings(guildID).WelcomeGifs)
}

// ErrWelcomeGifsFull is returned when the gif list is at its limit. Its text
// is shown to the administrator as it is.
var ErrWelcomeGifsFull = errors.New("the gif list is full (50) — remove one first")

// AddWelcomeGif adds a gif link, ignoring one already there.
func (s *Storage) AddWelcomeGif(guildID, url string) error {
	url = strings.TrimSpace(url)
	g := s.guildSettings(guildID)
	if slices.Contains(g.WelcomeGifs, url) {
		return nil
	}
	if len(g.WelcomeGifs) >= maxWelcomeGifs {
		return ErrWelcomeGifsFull
	}
	g.WelcomeGifs = append(g.WelcomeGifs, url)
	return s.settings.Put(g)
}

// RemoveWelcomeGif removes a gif link and reports whether it was there.
func (s *Storage) RemoveWelcomeGif(guildID, url string) (bool, error) {
	url = strings.TrimSpace(url)
	g := s.guildSettings(guildID)
	before := len(g.WelcomeGifs)
	g.WelcomeGifs = slices.DeleteFunc(g.WelcomeGifs, func(u string) bool { return u == url })
	if len(g.WelcomeGifs) == before {
		return false, nil
	}
	delete(g.WelcomeGifMedia, url)
	return true, s.settings.Put(g)
}

// SetWelcomeGifMedia records the gif file behind a gif link. A link that is
// not in the list is ignored: it was removed while being looked up.
func (s *Storage) SetWelcomeGifMedia(guildID, link, media string) error {
	link = strings.TrimSpace(link)
	g := s.guildSettings(guildID)
	if !slices.Contains(g.WelcomeGifs, link) {
		return nil
	}
	if g.WelcomeGifMedia == nil {
		g.WelcomeGifMedia = map[string]string{}
	}
	g.WelcomeGifMedia[link] = media
	return s.settings.Put(g)
}

// WelcomeGifMedia is the gif file behind a gif link, or "" when it has not
// been found.
func (s *Storage) WelcomeGifMedia(guildID, link string) string {
	return s.guildSettings(guildID).WelcomeGifMedia[strings.TrimSpace(link)]
}
