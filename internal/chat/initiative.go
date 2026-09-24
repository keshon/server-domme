package chat

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/memory"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// Starting things. The code finds openings and keeps the limits; whether any
// is worth acting on is hers to decide, with a reason she remembers. See
// mind.Mind.Initiate.
//
// These are safety limits, not a personality. v1 had a personality made of
// them — cooldowns learned from how welcome she was, fatigue, longing curves
// — and it still sent a hostile ping out of nowhere, because none of it
// carried a reason.
const (
	// lifeInterval is how often she looks around, varied by lifeJitter
	// either way so it never lands on the same minute past the hour.
	lifeInterval = 10 * time.Minute
	lifeJitter   = 1.0 / 3
	// lifeChance is the odds she considers anything on a given look. Most
	// looks cost nothing; the ones that do cost one call, and usually end
	// in her deciding to do nothing.
	lifeChance = 0.35
	// quietFrom and quietUntil are the night, in the community's timezone.
	// Nothing she starts arrives then.
	quietFrom  = 23
	quietUntil = 9
	// startsPerDay caps what she starts in a guild in a day, everything
	// counted.
	startsPerDay = 5
	// startGap is the least time between two things she starts in a guild.
	startGap = 45 * time.Minute
	// roomQuiet is how long a channel has to have been silent before
	// speaking into it is an opening at all, and roomStartsPerDay how often
	// one channel gets one.
	roomQuiet        = 90 * time.Minute
	roomStartsPerDay = 2
	// reachAfter is how long since someone last talked to her before going
	// to them is an opening, reachGap the least time between two reaches to
	// the same person, and reachUnanswered how many unanswered stop her
	// until they speak to her again.
	reachAfter      = 6 * time.Hour
	reachGap        = 4 * time.Hour
	reachUnanswered = 2
	// activeWithin is how recently someone must have been seen for her to
	// count them as around.
	activeWithin = 30 * time.Minute
	// maxOpenings is how many openings go in front of her at once.
	maxOpenings = 5
	// activityEvery throttles recording that an opted-in person was active
	// elsewhere, so a busy member is one write a minute, not one a message.
	activityEvery = time.Minute
)

// lifeState is what she started today in one guild.
type lifeState struct {
	day   string
	count int
	last  time.Time
}

// lifeLoop looks around now and then.
func (s *Service) lifeLoop(ctx context.Context) {
	for {
		wait := time.Duration(float64(lifeInterval) * (1 + lifeJitter*(2*s.roll()-1)))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.lookAround(ctx)
		}
	}
}

// lookAround considers every guild she listens in. It is also when what
// she started is settled and the rooms' counts are written; see welcome.go.
func (s *Service) lookAround(ctx context.Context) {
	s.sweepStarted(s.now())
	s.flushRooms()
	byGuild := make(map[string][]string)
	for channelID, guildID := range s.store.AllChatChannels() {
		byGuild[guildID] = append(byGuild[guildID], channelID)
	}
	for guildID, channels := range byGuild {
		if ctx.Err() != nil {
			return
		}
		sort.Strings(channels)
		s.considerStarting(ctx, guildID, channels)
	}
}

// considerStarting finds what she could start in a guild and asks her.
func (s *Service) considerStarting(ctx context.Context, guildID string, channels []string) {
	if s.impulses && s.idleMind {
		// What she starts comes from the idle mind now, not from a timer
		// finding a gap; see idle.go.
		return
	}
	now := s.now().In(s.location)
	if !s.awakeToStart(now) {
		return
	}
	if !s.mayStart(guildID, now) || s.roll() >= lifeChance {
		return
	}
	sess := s.session()
	if sess == nil {
		return
	}
	for _, c := range channels {
		s.backfill(sess, c)
	}

	openings := s.openings(sess, guildID, channels, now)
	if len(openings) == 0 {
		return
	}

	base := mind.Scene{
		GuildID:  guildID,
		Brief:    s.store.GetChatBrief(guildID),
		SelfName: s.DisplayName(sess, guildID),
		Now:      now,
		QuietFor: s.quietFor(guildID, now),
	}
	if guild, err := sess.State.Guild(guildID); err == nil && guild != nil {
		base.GuildName = guild.Name
	}
	var ids []string
	for _, o := range openings {
		ids = append(ids, o.UserID)
	}
	known, err := s.mind.Know(base, ids...)
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_memory_read_failed")
	}

	genCtx, cancel := context.WithTimeout(ctx, s.generateTimeout)
	defer cancel()
	plan, err := s.mind.Initiate(genCtx, base, known, openings)
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_initiative_failed")
		return
	}
	if plan.Choice < 0 {
		s.log.Debug().Str("guild_id", guildID).Int("openings", len(openings)).Msg("chat_initiative_declined")
		return
	}
	s.start(genCtx, sess, base, openings[plan.Choice], plan)
}

// start says what she decided to start, and remembers why.
func (s *Service) start(ctx context.Context, sess *discordgo.Session, base mind.Scene, o mind.Opening, plan mind.Plan) {
	if s.closed(sess, o.ChannelID) {
		s.log.Info().Str("channel_id", o.ChannelID).Msg("chat_start_restricted")
		return
	}
	if !s.droppedAt(base.GuildID, o.UserID, s.now()).IsZero() {
		// They asked her to drop something. Going to them is not dropping
		// it, whatever she means to say.
		s.log.Info().Str("guild_id", base.GuildID).Msg("chat_start_after_drop")
		return
	}
	sc := base
	sc.ChannelID, sc.ChannelName, sc.Trigger = o.ChannelID, o.ChannelName, o.Trigger
	sc.UserID, sc.Username = o.UserID, o.Username
	sc.Turns = s.conv.Recent(o.ChannelID)
	if channel, err := sess.State.Channel(o.ChannelID); err == nil && channel != nil {
		sc.ChannelTopic = channel.Topic
	}
	sc.Roles = s.roleNotes(sess, sc.GuildID, sc)

	entry := storage.MindJournal{
		GuildID: sc.GuildID, ChannelID: sc.ChannelID, At: sc.Now,
		UserID: sc.UserID, Username: sc.Username, Trigger: string(sc.Trigger),
		Act: string(mind.ActReply), Intent: plan.Intent, Why: plan.Why, Backend: plan.Backend,
	}
	defer func() { s.journal(entry) }()

	known, err := s.mind.Know(sc)
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", sc.GuildID).Msg("chat_memory_read_failed")
	}
	a := mind.Appraisal{Act: mind.ActReply, Intent: plan.Intent}
	s.drain(sc)
	reply, backend, err := s.speak(ctx, sc, known, a, plan.Why)
	if backend != "" {
		entry.Backend = backend
	}
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", sc.GuildID).Msg("chat_speak_failed")
		entry.Outcome, entry.Reason = outcomeDropped, err.Error()
		return
	}
	if err := sess.ChannelTyping(sc.ChannelID); err != nil {
		s.log.Debug().Err(err).Str("channel_id", sc.ChannelID).Msg("chat_typing_failed")
	}
	sent, err := s.deliver(ctx, sess, sc, reply, s.now())
	if err != nil {
		s.log.Warn().Err(err).Str("channel_id", sc.ChannelID).Msg("chat_send_failed")
		entry.Outcome, entry.Reason = outcomeDropped, "Discord refused the message: "+err.Error()
		return
	}
	entry.Outcome, entry.Posted, entry.ReplyID = outcomeAnswered, excerpt(sent.text, journalReply), sent.id

	if err := s.mind.Said(sc, a, sent.text, plan.Why, sent.id); err != nil {
		s.log.Warn().Err(err).Str("guild_id", sc.GuildID).Msg("chat_memory_write_failed")
	}
	form := formStart
	if sc.Trigger == mind.TriggerReach {
		form = formReach
	}
	s.watchStarted(sc, form, sent.id, sent.text)
	if o.Thread != nil {
		if err := s.memory.CloseThread(sc.GuildID, o.Thread.Key()); err != nil {
			s.log.Warn().Err(err).Str("guild_id", sc.GuildID).Msg("chat_memory_write_failed")
		}
	}
	day := sc.Now.Format("2006-01-02")
	if sc.Trigger == mind.TriggerReach {
		if err := s.store.MarkReached(sc.GuildID, sc.UserID, day, sc.Now); err != nil {
			s.log.Warn().Err(err).Str("guild_id", sc.GuildID).Msg("chat_reach_record_failed")
		}
	} else if err := s.store.MarkVolunteered(sc.GuildID, sc.ChannelID, day, sc.Now); err != nil {
		s.log.Warn().Err(err).Str("guild_id", sc.GuildID).Msg("chat_start_record_failed")
	}
	s.started(sc.GuildID, sc.Now)
	s.log.Info().
		Str("guild_id", sc.GuildID).
		Str("channel_id", sc.ChannelID).
		Str("trigger", string(sc.Trigger)).
		Msg("chat_started")
}

// openings are what she could start: intentions come due, people who let her
// come to them and have been away, and quiet rooms she may speak up in.
func (s *Service) openings(sess *discordgo.Session, guildID string, channels []string, now time.Time) []mind.Opening {
	var out []mind.Opening
	reachOff := s.store.IsChatAttentionOff(guildID)
	names := channelNames(sess, channels)
	reachable := make(map[string]storage.MindPerson)
	if !reachOff {
		for _, p := range s.store.AttentionSeekers(guildID) {
			reachable[p.UserID] = p
		}
	}

	threads, err := s.memory.Threads(guildID)
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_memory_read_failed")
	}
	claimed := make(map[string]bool)
	for _, t := range memory.Unfinished(threads) {
		if t.Due.After(now) {
			continue
		}
		th := t
		if p, ok := reachable[t.Person.ID]; ok && s.mayReach(p, now) {
			c := reachChannel(p, channels)
			out = append(out, mind.Opening{
				Trigger: mind.TriggerReach, ChannelID: c, ChannelName: names[c],
				UserID: p.UserID, Username: nameOr(p.Username, t.Person.Name), Thread: &th,
				Detail: lastTalked(p, now), Turns: s.conv.Recent(c),
			})
			claimed[p.UserID] = true
			continue
		}
		// Not someone she may go after: she can still bring it up where
		// they last talked to her, if she may speak up there.
		c := ""
		if p := s.store.GetMindPerson(guildID, t.Person.ID); p != nil {
			c = p.LastChatChannel
		}
		if !s.store.IsChatProactive(guildID, c) {
			c = firstProactive(s.store, guildID, channels)
		}
		if c != "" {
			out = append(out, mind.Opening{
				Trigger: mind.TriggerStart, ChannelID: c, ChannelName: names[c],
				UserID: t.Person.ID, Username: t.Person.Name, Thread: &th, Turns: s.conv.Recent(c),
			})
		}
	}

	for _, p := range reachable {
		if claimed[p.UserID] || !s.mayReach(p, now) || now.Sub(p.LastExchangeAt) < reachAfter {
			continue
		}
		c := reachChannel(p, channels)
		out = append(out, mind.Opening{
			Trigger: mind.TriggerReach, ChannelID: c, ChannelName: names[c],
			UserID: p.UserID, Username: p.Username, Detail: lastTalked(p, now), Turns: s.conv.Recent(c),
		})
	}

	for _, c := range channels {
		if !s.store.IsChatProactive(guildID, c) {
			continue
		}
		st := s.store.MindChannelState(guildID, c)
		if st.Day == now.Format("2006-01-02") && st.Today >= roomStartsPerDay {
			continue
		}
		turns := s.conv.Recent(c)
		quiet := "It has been quiet there for a long while."
		if len(turns) > 0 {
			since := now.Sub(turns[len(turns)-1].At)
			if since < roomQuiet {
				continue
			}
			quiet = fmt.Sprintf("Nobody has said anything there for %s.", roughly(since))
		}
		out = append(out, mind.Opening{
			Trigger: mind.TriggerStart, ChannelID: c, ChannelName: names[c], Detail: quiet, Turns: turns,
		})
	}

	if len(out) > maxOpenings {
		out = out[:maxOpenings]
	}
	return out
}

// quietFor is how long since anyone spoke to her in a guild, or zero when
// nobody has since the bot started: not known is not the same as long.
func (s *Service) quietFor(guildID string, now time.Time) time.Duration {
	s.approachMu.Lock()
	defer s.approachMu.Unlock()
	at, ok := s.approached[guildID]
	if !ok {
		return 0
	}
	return now.Sub(at)
}

// awakeToStart reports whether she is in a state to start anything: with a
// body, online and with some energy left; without one, outside the quiet
// hours, as in v2.
func (s *Service) awakeToStart(local time.Time) bool {
	if s.body != nil {
		return s.online() && s.battery() > batteryLow
	}
	h := local.Hour()
	return h < quietFrom && h >= quietUntil
}

// mayReach reports whether going after someone is allowed at all right now:
// not too soon after the last time, not after they left two unanswered, and
// not while they are already talking to her.
func (s *Service) mayReach(p storage.MindPerson, now time.Time) bool {
	if p.Unanswered >= reachUnanswered {
		return false
	}
	if !p.ReachedAt.IsZero() && now.Sub(p.ReachedAt) < reachGap {
		return false
	}
	return now.Sub(p.LastExchangeAt) >= activeWithin
}

// mayStart reports whether the day's limits allow her to start anything more
// in a guild.
func (s *Service) mayStart(guildID string, now time.Time) bool {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	st := s.life[guildID]
	if st == nil || st.day != now.Format("2006-01-02") {
		return true
	}
	return st.count < startsPerDay*startUnits && now.Sub(st.last) >= startGap
}

// started counts one more thing started today.
func (s *Service) started(guildID string, now time.Time) {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	day := now.Format("2006-01-02")
	st := s.life[guildID]
	if st == nil || st.day != day {
		st = &lifeState{day: day}
		s.life[guildID] = st
	}
	st.count += startUnits
	st.last = now
}

// startUnits is what one start counts toward the day's limit; a reaction
// she starts counts one unit, a third of one. See docs/persona-v3.md, H6.
const startUnits = 3

// withdrawConsent ends someone's consent to be reached, because they asked
// her to leave them alone. See mind.Appraisal.BackOff.
func (s *Service) withdrawConsent(guildID, userID string) {
	p := s.store.GetMindPerson(guildID, userID)
	if p == nil || !Consented(p.Attention) {
		return
	}
	if err := s.store.SetMindConsent(guildID, userID, "", s.now()); err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_attention_withdraw_failed")
		return
	}
	s.log.Info().Str("guild_id", guildID).Str("user_id", userID).Msg("chat_attention_withdrawn")
}

// noteActivity records, for someone who opted in, that they were active in a
// channel she does not read: a timestamp only, nothing about where or what.
func (s *Service) noteActivity(m *discordgo.MessageCreate) {
	p := s.store.GetMindPerson(m.GuildID, m.Author.ID)
	if p == nil || !Consented(p.Attention) {
		return
	}
	now := s.now()
	if now.Sub(p.LastActiveAt) < activityEvery {
		return
	}
	if err := s.store.ActiveMindPerson(m.GuildID, m.Author.ID, now); err != nil {
		s.log.Debug().Err(err).Str("guild_id", m.GuildID).Msg("chat_activity_record_failed")
	}
}

// ConsentOn is the value /attention stores for someone who agreed to be
// reached.
const ConsentOn = "on"

// Consented reports whether a stored consent value is a yes.
func Consented(v string) bool { return v == ConsentOn }

// reachChannel is where to find someone: where they last talked to her, if
// she still listens there, or else the first channel she listens in.
func reachChannel(p storage.MindPerson, channels []string) string {
	for _, c := range channels {
		if c == p.LastChatChannel {
			return c
		}
	}
	if len(channels) > 0 {
		return channels[0]
	}
	return ""
}

func firstProactive(store *storage.Storage, guildID string, channels []string) string {
	for _, c := range channels {
		if store.IsChatProactive(guildID, c) {
			return c
		}
	}
	return ""
}

// lastTalked describes how long someone has been away from her, and whether
// they are around.
func lastTalked(p storage.MindPerson, now time.Time) string {
	var out string
	if p.LastExchangeAt.IsZero() {
		out = "They have never really talked to her."
	} else {
		out = fmt.Sprintf("They last talked to her %s ago.", roughly(now.Sub(p.LastExchangeAt)))
	}
	if seen := latest(p.LastSeen, p.LastActiveAt); !seen.IsZero() && now.Sub(seen) < activeWithin {
		out += " They are around in the server right now."
	}
	if p.Unanswered > 0 {
		out += " They have not answered the last time she came to them."
	}
	return out
}

func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// roughly renders a duration the way a person would.
func roughly(d time.Duration) string {
	switch {
	case d < 2*time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	}
}

func channelNames(sess *discordgo.Session, channels []string) map[string]string {
	out := make(map[string]string, len(channels))
	for _, c := range channels {
		// Never the id: it would go in front of the model as a name.
		out[c] = "a channel"
		if sess != nil && sess.State != nil {
			if ch, err := sess.State.Channel(c); err == nil && ch != nil {
				out[c] = ch.Name
			}
		}
	}
	return out
}

func nameOr(name, fallback string) string {
	if name != "" {
		return name
	}
	return fallback
}
