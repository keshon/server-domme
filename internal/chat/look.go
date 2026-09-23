package chat

import (
	"context"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
)

// Going to look, when she asks. Her walks come on the idle mind's clock;
// this is the same thing on her own account, in the middle of a
// conversation: "hang on" — she puts her head into a room she reads, and
// comes back with the gist before she answers.
//
// The model chooses the room; the code keeps the gate. Looking costs her a
// pause and comes out of a small daily budget, because perception that
// costs nothing turns her into a search engine with a character card. See
// docs/persona-v3.md, D.
const (
	// looksPerDay is how often she may go and look in a guild in a day.
	looksPerDay = 3
	// lookGap is how soon she may look into the same room again: a walk
	// through it counts, so the idle mind and this share one clock.
	lookGap = 20 * time.Minute
	// lookPause is how long she is away doing it, before she answers.
	lookPauseMin = 15 * time.Second
	lookPauseMax = 45 * time.Second
)

// lookState counts the looks she has taken in a guild on a day.
type lookState struct {
	day string
	n   int
}

// readNames are the rooms she passes through and never speaks in, by name.
func (s *Service) readNames(sess *discordgo.Session, guildID string) []string {
	var open []string
	for _, c := range s.store.GetChatReads(guildID) {
		if !s.closed(sess, c) {
			open = append(open, c)
		}
	}
	if len(open) == 0 {
		return nil
	}
	names := channelNames(sess, open)
	out := make([]string, 0, len(open))
	for _, c := range open {
		if name := names[c]; name != "" {
			out = append(out, name)
		}
	}
	return out
}

// lookAt takes one look into a room she reads, when the rails allow. It
// returns what she took from it, or why she could not go.
func (s *Service) lookAt(ctx context.Context, sess *discordgo.Session, sc mind.Scene, want string) (string, string) {
	if !s.walks || !s.looks {
		return "", "looking is switched off"
	}
	channelID := s.readChannelNamed(sess, sc.GuildID, want)
	if channelID == "" {
		return "", "she asked to look at #" + want + ", which is not a room she reads"
	}
	now := s.now()
	s.idleMu.Lock()
	last := s.walked[channelID]
	s.idleMu.Unlock()
	if !last.IsZero() && now.Sub(last) < lookGap {
		return "", "she was just in #" + want
	}
	if !s.mayLook(sc.GuildID, now) {
		return "", "she has been in and out of the rooms she reads enough today"
	}

	var fresh []mind.Turn
	for _, t := range s.conv.Recent(channelID) {
		if t.At.After(last) && !t.FromBot {
			fresh = append(fresh, t)
		}
	}
	if len(fresh) > walkLines {
		fresh = fresh[len(fresh)-walkLines:]
	}

	// Away for a moment, the way anyone is who says "hang on".
	if !s.sleep(ctx, lookPauseMin+time.Duration(s.roll()*float64(lookPauseMax-lookPauseMin))) {
		return "", ""
	}
	s.idleMu.Lock()
	s.walked[channelID] = now
	s.idleMu.Unlock()
	s.looked(sc.GuildID, now)

	caught, err := s.mind.Glance(ctx, sc, mind.Walk{Channel: want, Lines: fresh})
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", sc.GuildID).Msg("chat_look_failed")
		return "", "she went to look at #" + want + " and the backends would not say what she saw"
	}
	s.log.Info().
		Str("guild_id", sc.GuildID).
		Str("channel", want).
		Int("lines", len(fresh)).
		Bool("caught", caught != "").
		Msg("chat_looked")
	return caught, ""
}

// readChannelNamed is the channel she reads that goes by a name, or "".
func (s *Service) readChannelNamed(sess *discordgo.Session, guildID, want string) string {
	want = strings.TrimPrefix(strings.TrimSpace(want), "#")
	reads := s.store.GetChatReads(guildID)
	names := channelNames(sess, reads)
	for _, c := range reads {
		if strings.EqualFold(names[c], want) && !s.closed(sess, c) {
			return c
		}
	}
	return ""
}

// mayLook reports whether the day's budget allows another look.
func (s *Service) mayLook(guildID string, now time.Time) bool {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	st := s.lookCounts[guildID]
	return st == nil || st.day != now.Format("2006-01-02") || st.n < looksPerDay
}

// looked counts one look.
func (s *Service) looked(guildID string, now time.Time) {
	day := now.Format("2006-01-02")
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	st := s.lookCounts[guildID]
	if st == nil || st.day != day {
		st = &lookState{day: day}
		s.lookCounts[guildID] = st
	}
	st.n++
}
