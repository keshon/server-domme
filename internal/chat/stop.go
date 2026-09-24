package chat

import (
	"time"

	"github.com/keshon/server-domme/internal/mind"
)

// Being asked to stop. Someone says "drop it", "I want out", "let's change
// the subject" — and that is the end of it, in code rather than in a rule
// the model may read past.
//
// In production she answered six of those in a row by calling each one a
// dodge, and pressed for forty minutes: "youre not walking away from this
// again", "sleep well - i'll be here if you meant it". Nothing stopped her,
// because every rail until now was built for the opposite failure — v1 and
// v2 went quiet when they should have answered. This is the other end.
const (
	// droppedFor is how long after being asked to drop something she
	// leaves that person alone: no starting anything with them, no second
	// thoughts at them, and the fact in front of her when she answers.
	droppedFor = 12 * time.Hour
	// pressLimit is how many times she may put the same ask to someone
	// before the code lets it go for her; the one after that is not sent.
	// pressWithin is how long that counting lasts.
	pressLimit  = 2
	pressWithin = 30 * time.Minute
)

// dropIntent is what she has to get across once somebody has asked her to
// stop: the ask she was carrying is exactly the thing they asked her to stop
// making, so it does not survive into the message.
const dropIntent = "let it go, briefly and without an argument, and leave them be"

// pressState is the asks she has put to one person in one room lately.
type pressState struct {
	asks []string
	at   time.Time
}

// dropIt records that someone asked her to stop, and closes what she meant
// to do about them: the intentions are what keep her coming back.
func (s *Service) dropIt(guildID, userID string, now time.Time) {
	if userID == "" {
		return
	}
	s.dropMu.Lock()
	s.dropped[guildID+"|"+userID] = now
	s.dropMu.Unlock()

	closed, err := s.memory.CloseThreadsAbout(guildID, userID)
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_memory_write_failed")
	}
	s.log.Info().Str("guild_id", guildID).Int("closed", closed).Msg("chat_asked_to_drop_it")
}

// droppedAt is when someone last asked her to drop something, while it is
// recent enough to still hold.
func (s *Service) droppedAt(guildID, userID string, now time.Time) time.Time {
	if userID == "" {
		return time.Time{}
	}
	s.dropMu.Lock()
	defer s.dropMu.Unlock()
	at := s.dropped[guildID+"|"+userID]
	if at.IsZero() || now.Sub(at) > droppedFor {
		return time.Time{}
	}
	return at
}

// pressing records the gist she is about to put to someone and reports
// whether it is the same ask she has already put to them pressLimit times.
// Her wording changes while the demand does not, so the gists are compared
// as asks rather than as strings.
func (s *Service) pressing(guildID, channelID, userID, intent string, now time.Time) bool {
	if intent == "" || userID == "" {
		return false
	}
	key := guildID + "|" + channelID + "|" + userID
	s.dropMu.Lock()
	defer s.dropMu.Unlock()
	st := s.presses[key]
	if st == nil || now.Sub(st.at) > pressWithin {
		st = &pressState{}
		s.presses[key] = st
	}
	st.at = now
	same := 0
	for _, earlier := range st.asks {
		if mind.SameAsk(earlier, intent) {
			same++
		}
	}
	st.asks = append(st.asks, intent)
	if len(st.asks) > pressLimit {
		st.asks = st.asks[len(st.asks)-pressLimit:]
	}
	return same >= pressLimit
}
