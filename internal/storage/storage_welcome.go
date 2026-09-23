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

	// Gifs are the role's own links for its welcomes to pick from. None
	// means the guild's shared pool is used instead.
	Gifs []string `json:"gifs,omitempty"`
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
		to.Gifs = slices.Clone(from.Gifs)
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

// WelcomeGifs lists a pool of welcome gifs: a role's own, or with roleID ""
// the guild's shared pool.
func (s *Storage) WelcomeGifs(guildID, roleID string) []string {
	if roleID == "" {
		return slices.Clone(s.guildSettings(guildID).WelcomeGifs)
	}
	if w := s.WelcomeRoleFor(guildID, roleID); w != nil {
		return slices.Clone(w.Gifs)
	}
	return nil
}

// WelcomeGifPool is what a role's welcomes pick from: its own gifs, or the
// shared pool when it has none, and which of the two it is.
func (s *Storage) WelcomeGifPool(guildID, roleID string) (gifs []string, shared bool) {
	if own := s.WelcomeGifs(guildID, roleID); len(own) > 0 {
		return own, false
	}
	return s.WelcomeGifs(guildID, ""), true
}

// ErrWelcomeGifsFull is returned when a gif list is at its limit. Its text
// is shown to the administrator as it is.
var ErrWelcomeGifsFull = errors.New("the gif list is full (50) — remove one first")

// AddWelcomeGif adds a gif link to a role's pool, or with roleID "" to the
// shared one, ignoring one already there.
func (s *Storage) AddWelcomeGif(guildID, roleID, url string) error {
	url = strings.TrimSpace(url)
	add := func(list []string) ([]string, error) {
		if slices.Contains(list, url) {
			return list, nil
		}
		if len(list) >= maxWelcomeGifs {
			return list, ErrWelcomeGifsFull
		}
		return append(list, url), nil
	}
	if roleID != "" {
		var full error
		err := s.UpdateWelcomeRole(guildID, roleID, func(w *WelcomeRole) {
			w.Gifs, full = add(w.Gifs)
		})
		if full != nil {
			return full
		}
		return err
	}
	g := s.guildSettings(guildID)
	list, err := add(g.WelcomeGifs)
	if err != nil {
		return err
	}
	g.WelcomeGifs = list
	return s.settings.Put(g)
}

// RemoveWelcomeGif removes a gif link from a role's pool, or with roleID ""
// from the shared one, and reports whether it was there.
func (s *Storage) RemoveWelcomeGif(guildID, roleID, url string) (bool, error) {
	url = strings.TrimSpace(url)
	if !slices.Contains(s.WelcomeGifs(guildID, roleID), url) {
		return false, nil
	}
	drop := func(u string) bool { return u == url }
	if roleID != "" {
		if err := s.UpdateWelcomeRole(guildID, roleID, func(w *WelcomeRole) {
			w.Gifs = slices.DeleteFunc(w.Gifs, drop)
		}); err != nil {
			return false, err
		}
	} else {
		g := s.guildSettings(guildID)
		g.WelcomeGifs = slices.DeleteFunc(g.WelcomeGifs, drop)
		if err := s.settings.Put(g); err != nil {
			return false, err
		}
	}
	// The file behind it is kept while another pool still has the link.
	if !s.welcomeGifListed(guildID, url) {
		g := s.guildSettings(guildID)
		if _, ok := g.WelcomeGifMedia[url]; ok {
			delete(g.WelcomeGifMedia, url)
			return true, s.settings.Put(g)
		}
	}
	return true, nil
}

// welcomeGifListed reports whether any pool in the guild has a link.
func (s *Storage) welcomeGifListed(guildID, link string) bool {
	if slices.Contains(s.guildSettings(guildID).WelcomeGifs, link) {
		return true
	}
	for _, w := range s.WelcomeRoles(guildID) {
		if slices.Contains(w.Gifs, link) {
			return true
		}
	}
	return false
}

// SetWelcomeGifMedia records the gif file behind a gif link, for every pool
// that has it. A link no pool has is ignored: it was removed while being
// looked up.
func (s *Storage) SetWelcomeGifMedia(guildID, link, media string) error {
	link = strings.TrimSpace(link)
	if !s.welcomeGifListed(guildID, link) {
		return nil
	}
	g := s.guildSettings(guildID)
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
