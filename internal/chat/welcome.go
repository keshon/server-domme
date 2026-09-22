package chat

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/memory"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// Whether she was welcome. Everything she starts — going to someone, speaking
// up in a room, following up when someone turns up, joining a conversation —
// is watched for what became of it: answered, reacted to, or ignored, and
// after how long. The outcome goes to the journal and, as something that
// happened, into her memory, where reflection reads it. No rate is ever
// shown to her. See docs/persona-v3.md, H5.

// Forms of starting something, as the journal and the logs name them.
const (
	formReach = "reach"
	formStart = "start"
	formSight = "sight"
	formJoin  = "join"
)

// Outcomes.
const (
	receivedAnswered = "answered"
	receivedReacted  = "reacted"
	receivedIgnored  = "ignored"
)

const (
	// welcomeWindow is how long something she started waits for a response
	// before it counts as ignored.
	welcomeWindow = 2 * time.Hour
	// maxStarted bounds what is watched at once.
	maxStarted = 64
	// roomReplyWindow is how soon someone else has to speak after a message
	// for it to count as answered, for the room's base rate.
	roomReplyWindow = 10 * time.Minute
	// receivedWeight is how much a response to something she started stays
	// with her: a little more than small talk, because it says something
	// about how she is received.
	receivedWeight = 0.3
)

// startedMsg is something she started, waiting to see how it lands.
type startedMsg struct {
	guildID, channelID, channelName, id string
	form                                string
	// to is who it was aimed at; empty means the room.
	to, toName string
	text       string
	at         time.Time
	// reactedBy and reactedAt are the first reaction from the right person.
	reactedBy string
	reactedAt time.Time
}

// watchStarted records something she started, to see what becomes of it.
func (s *Service) watchStarted(sc mind.Scene, form, messageID, text string) {
	if messageID == "" {
		return
	}
	t := &startedMsg{
		guildID: sc.GuildID, channelID: sc.ChannelID, channelName: sc.ChannelName, id: messageID,
		form: form, text: text, at: s.now(),
	}
	if form != formStart {
		t.to, t.toName = sc.UserID, sc.Username
	}
	s.startedMu.Lock()
	defer s.startedMu.Unlock()
	if len(s.startedMsgs) >= maxStarted {
		var oldest *startedMsg
		for _, o := range s.startedMsgs {
			if oldest == nil || o.at.Before(oldest.at) {
				oldest = o
			}
		}
		delete(s.startedMsgs, oldest.id)
	}
	s.startedMsgs[messageID] = t
}

// noticeResponse checks a new message against what she started in its
// channel: a reply to it, the person it was aimed at speaking, or — for
// something said to the room — anyone speaking.
func (s *Service) noticeResponse(m *discordgo.MessageCreate, name string, now time.Time) {
	var answered []*startedMsg
	s.startedMu.Lock()
	for id, t := range s.startedMsgs {
		if t.channelID != m.ChannelID || !now.After(t.at) {
			continue
		}
		replied := m.MessageReference != nil && m.MessageReference.MessageID == id
		if replied || (t.to != "" && m.Author.ID == t.to) || t.to == "" {
			answered = append(answered, t)
			delete(s.startedMsgs, id)
		}
	}
	s.startedMu.Unlock()
	for _, t := range answered {
		s.received(t, receivedAnswered, name, now)
	}
}

// ObserveReaction takes a reaction to any message the bot can see; only
// reactions to something she started matter.
func (s *Service) ObserveReaction(r *discordgo.MessageReaction) {
	if r == nil {
		return
	}
	s.startedMu.Lock()
	defer s.startedMu.Unlock()
	t, ok := s.startedMsgs[r.MessageID]
	if !ok || !t.reactedAt.IsZero() || (t.to != "" && r.UserID != t.to) {
		return
	}
	t.reactedBy, t.reactedAt = r.UserID, s.now()
}

// sweepStarted settles whatever has waited out the window: reacted to, or
// ignored.
func (s *Service) sweepStarted(now time.Time) {
	var due []*startedMsg
	s.startedMu.Lock()
	for id, t := range s.startedMsgs {
		if now.Sub(t.at) >= welcomeWindow {
			due = append(due, t)
			delete(s.startedMsgs, id)
		}
	}
	s.startedMu.Unlock()
	sort.Slice(due, func(i, j int) bool { return due[i].at.Before(due[j].at) })
	for _, t := range due {
		if !t.reactedAt.IsZero() {
			s.received(t, receivedReacted, "", t.reactedAt)
		} else {
			s.received(t, receivedIgnored, "", now)
		}
	}
}

// received records what became of something she started: in the journal,
// in the log, and in her memory as something that happened.
func (s *Service) received(t *startedMsg, outcome, by string, at time.Time) {
	after := at.Sub(t.at)
	for _, e := range s.store.MindJournalIn(t.guildID, t.channelID) {
		if e.ReplyID != t.id {
			continue
		}
		err := s.store.UpdateMindJournal(t.guildID, e.ID, func(j *storage.MindJournal) {
			j.Received, j.ReceivedAfter = outcome, after
		})
		if err != nil {
			s.log.Debug().Err(err).Str("guild_id", t.guildID).Msg("chat_journal_write_failed")
		}
		break
	}
	s.log.Info().
		Str("guild_id", t.guildID).
		Str("channel_id", t.channelID).
		Str("form", t.form).
		Str("received", outcome).
		Dur("after", after).
		Msg("chat_started_received")

	var people []memory.Ref
	if t.to != "" {
		people = []memory.Ref{{ID: t.to, Name: t.toName}}
	}
	err := s.memory.AddMoment(t.guildID, memory.Moment{
		At: at, Channel: t.channelName, People: people, Weight: receivedWeight,
		Text: receivedLine(t, outcome, by, after),
	})
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", t.guildID).Msg("chat_memory_write_failed")
	}
}

// receivedLine is what became of something she started, as a line in her
// day: what she did, and what came of it. A fact, not a verdict.
func receivedLine(t *startedMsg, outcome, by string, after time.Duration) string {
	said := fmt.Sprintf("%q", clipText(t.text, 120))
	var did string
	switch t.form {
	case formReach:
		did = fmt.Sprintf("I went to %s and said %s", nameOr(t.toName, "someone"), said)
	case formSight:
		did = fmt.Sprintf("I brought something up with %s: %s", nameOr(t.toName, "someone"), said)
	case formJoin:
		did = fmt.Sprintf("I joined in with %s: %s", nameOr(t.toName, "someone"), said)
	default:
		did = fmt.Sprintf("I spoke up in #%s: %s", nameOr(t.channelName, "a channel"), said)
	}
	switch outcome {
	case receivedAnswered:
		who := nameOr(by, nameOr(t.toName, "someone"))
		return fmt.Sprintf("%s — %s answered %s later", did, who, roughly(after))
	case receivedReacted:
		return did + " — it got a reaction, and no answer"
	default:
		if t.to != "" {
			return fmt.Sprintf("%s — %s did not answer", did, nameOr(t.toName, "they"))
		}
		return did + " — nobody answered"
	}
}

func clipText(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// roomState is the last message in a channel, for the base rate.
type roomState struct {
	author   string
	at       time.Time
	answered bool
}

// roomCount is one channel's day so far: messages, and how many got an
// answer from someone else.
type roomCount struct {
	guildID, day, channelID string
	messages, answered      int
}

// noteRoom counts a message for its channel's base rate: how often anyone
// gets an answer there. Being ignored in a room where most messages go
// unanswered is the room, not her; reflection is told the rate so it can
// tell the difference. Counted in memory and flushed with the life loop, so
// the gateway goroutine does no extra write.
func (s *Service) noteRoom(guildID, channelID, authorID string, now time.Time) {
	day := now.In(s.location).Format("2006-01-02")
	s.roomMu.Lock()
	defer s.roomMu.Unlock()
	key := guildID + "|" + day + "|" + channelID
	c := s.roomCounts[key]
	if c == nil {
		c = &roomCount{guildID: guildID, day: day, channelID: channelID}
		s.roomCounts[key] = c
	}
	c.messages++
	last := s.rooms[channelID]
	if last != nil && !last.answered && last.author != authorID && now.Sub(last.at) <= roomReplyWindow &&
		last.at.In(s.location).Format("2006-01-02") == day {
		c.answered++
	}
	if last != nil && last.author != authorID {
		last.answered = true
	}
	s.rooms[channelID] = &roomState{author: authorID, at: now}
}

// flushRooms writes the counted base rates to the datastore.
func (s *Service) flushRooms() {
	s.roomMu.Lock()
	counts := s.roomCounts
	s.roomCounts = make(map[string]*roomCount)
	s.roomMu.Unlock()
	for _, c := range counts {
		for event, n := range map[string]int{roomEvent(c.channelID, "messages"): c.messages, roomEvent(c.channelID, "answered"): c.answered} {
			if err := s.store.AddMindEvents(c.guildID, c.day, event, n); err != nil {
				s.log.Debug().Err(err).Str("guild_id", c.guildID).Msg("chat_count_failed")
			}
		}
	}
}

// roomEvent names a channel's base-rate count in the day's counts.
func roomEvent(channelID, what string) string { return "room " + channelID + " " + what }

// roomRates are a guild's base rates for a day, by channel name, for
// reflection.
func (s *Service) roomRates(guildID string, date time.Time) []mind.RoomRate {
	s.flushRooms()
	counts := s.store.MindDayCounts(guildID, date.In(s.location).Format("2006-01-02"))
	var names map[string]string
	if sess := s.session(); sess != nil {
		names = channelNames(sess, s.store.GetChatChannels(guildID))
	}
	var out []mind.RoomRate
	for event, n := range counts {
		rest, ok := strings.CutPrefix(event, "room ")
		if !ok || !strings.HasSuffix(rest, " messages") || n == 0 {
			continue
		}
		channelID := strings.TrimSuffix(rest, " messages")
		answered := counts[roomEvent(channelID, "answered")]
		name := names[channelID]
		if name == "" {
			name = "a channel"
		}
		out = append(out, mind.RoomRate{Channel: name, Messages: n, Unanswered: max(0, n-answered)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Messages > out[j].Messages })
	return out
}

// afterUnprompted finishes something she said without being spoken to: a
// follow-up counts as a start and closes its intention, and both it and a
// conversation she joined are watched for how they land.
func (s *Service) afterUnprompted(sc mind.Scene, sent sentMessage) {
	switch sc.Trigger {
	case mind.TriggerSight:
		s.started(sc.GuildID, sc.Now)
		if sc.Thread != nil {
			if err := s.memory.CloseThread(sc.GuildID, sc.Thread.Key()); err != nil {
				s.log.Warn().Err(err).Str("guild_id", sc.GuildID).Msg("chat_memory_write_failed")
			}
		}
		s.watchStarted(sc, formSight, sent.id, sent.text)
	case mind.TriggerOverheard:
		s.watchStarted(sc, formJoin, sent.id, sent.text)
	}
}
