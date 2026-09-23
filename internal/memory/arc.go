package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Arc is how one conversation in one channel has gone so far, in her own
// words: what it has been about, what each side has done, and what she keeps
// doing.
//
// The transcript she is shown ends where the conversation stops being live —
// half an hour, or the last few lines — so a conversation that pauses for an
// afternoon comes back to her as a handful of lines with no history. In
// production that is how she asked the same question twice, three hours
// apart, and was told so. The arc is what carries the thread across the
// pause.
//
// One per channel, rewritten as the conversation moves, and closed into a
// moment of the day once it is over: from then on it is remembered, not
// in front of her. See docs/persona-v3.md.
type Arc struct {
	ChannelID string
	// Channel is the channel's name, for the moment it closes into.
	Channel string
	Started time.Time
	Updated time.Time
	// People are who took part, as she knew them.
	People []Ref
	Text   string
	// Source is always her interpretation: an arc is what she made of a
	// conversation, never a record of it.
	Source Source
}

// ArcWeight is the weight of the moment an arc closes into: enough to be
// recalled for a fortnight, and below LastingWeight, since the day's summary
// carries what outlasts that.
const ArcWeight = 0.4

// arcMomentPrefix opens the moment an arc closes into, so it reads as the
// shape of a conversation rather than as one thing said.
const arcMomentPrefix = "how the conversation went: "

// maxArcPeople bounds who an arc names. A room with more people than this in
// one conversation is a crowd, and the first few are who it was about.
const maxArcPeople = 6

// Arc front matter keys.
const (
	keyArcChannel = "channel"
	keyArcName    = "name"
	keyArcStarted = "started"
	keyArcUpdated = "updated"
	keyArcPeople  = "people"
	keyArcFrom    = "from"
)

// Arc reads the arc of a channel. The bool is false when there is none.
func (s *Store) Arc(guildID, channelID string) (Arc, bool, error) {
	path, err := s.arcPath(guildID, channelID)
	if err != nil {
		return Arc{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := readArc(path)
	return a, a.Text != "", err
}

// SetArc writes a channel's arc, replacing the one there.
func (s *Store) SetArc(guildID string, a Arc) error {
	path, err := s.arcPath(guildID, a.ChannelID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(a.Text) == "" {
		return fmt.Errorf("memory: an arc needs text")
	}
	if len(a.People) > maxArcPeople {
		a.People = a.People[:maxArcPeople]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeFile(path, renderArc(a))
}

// CloseArc ends a channel's arc: it becomes a moment of the day it was last
// touched, and the file goes. It reports false when there was none.
func (s *Store) CloseArc(guildID, channelID string) (bool, error) {
	return s.closeArcs(guildID, func(a Arc) bool { return a.ChannelID == channelID })
}

// CloseArcsBefore ends every arc last touched before a time, so a day she
// reflects on has its conversations in it. It reports how many it closed.
func (s *Store) CloseArcsBefore(guildID string, before time.Time) (int, error) {
	n := 0
	_, err := s.closeArcs(guildID, func(a Arc) bool {
		if a.Updated.Before(before) {
			n++
			return true
		}
		return false
	})
	return n, err
}

func (s *Store) closeArcs(guildID string, which func(Arc) bool) (bool, error) {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(filepath.Join(dir, arcsDir))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("memory: read %s: %w", arcsDir, err)
	}
	closed := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, arcsDir, e.Name())
		a, err := readArc(path)
		if err != nil {
			return closed, err
		}
		if a.Text == "" {
			// Nothing to carry over; a hand-emptied file just goes.
			_ = os.Remove(path)
			continue
		}
		if !which(a) {
			continue
		}
		if err := s.addMomentLocked(dir, a.moment()); err != nil {
			return closed, err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return closed, fmt.Errorf("memory: remove %s: %w", e.Name(), err)
		}
		closed = true
	}
	return closed, nil
}

// moment is what an arc leaves behind once it is over.
func (a Arc) moment() Moment {
	at := a.Updated
	if at.IsZero() {
		at = a.Started
	}
	return Moment{
		At: at, Channel: a.Channel, People: a.People,
		Text: arcMomentPrefix + a.Text, Weight: ArcWeight, Kind: Interpreted,
	}
}

func (s *Store) arcPath(guildID, channelID string) (string, error) {
	dir, err := s.guildDir(guildID)
	if err != nil {
		return "", err
	}
	if !safeName(channelID) {
		return "", fmt.Errorf("memory: bad channel id %q", channelID)
	}
	return filepath.Join(dir, arcsDir, channelID+".md"), nil
}

func readArc(path string) (Arc, error) {
	text, err := readFile(path)
	if err != nil || text == "" {
		return Arc{}, err
	}
	fields, body := parseDoc(text)
	a := Arc{
		ChannelID: fields[keyArcChannel],
		Channel:   fields[keyArcName],
		Started:   parseTime(fields[keyArcStarted]),
		Updated:   parseTime(fields[keyArcUpdated]),
		Text:      oneLine(body),
	}
	if a.ChannelID == "" {
		a.ChannelID = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	a.Source, _ = ParseSource(fields[keyArcFrom])
	for _, part := range strings.Split(fields[keyArcPeople], ";") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		if i := strings.LastIndex(part, ":"); i > 0 {
			a.People = append(a.People, Ref{Name: strings.TrimSpace(part[:i]), ID: strings.TrimSpace(part[i+1:])})
		} else {
			a.People = append(a.People, Ref{Name: part})
		}
	}
	return a, nil
}

func renderArc(a Arc) string {
	var people []string
	for _, p := range a.People {
		name := refName(p.Name)
		switch {
		case p.ID != "":
			people = append(people, name+":"+p.ID)
		case name != "":
			people = append(people, name)
		}
	}
	return renderDoc([]field{
		{keyArcChannel, a.ChannelID},
		{keyArcName, a.Channel},
		{keyArcStarted, formatTime(a.Started)},
		{keyArcUpdated, formatTime(a.Updated)},
		{keyArcPeople, strings.Join(people, "; ")},
		{keyArcFrom, a.Source.String()},
	}, oneLine(a.Text))
}
