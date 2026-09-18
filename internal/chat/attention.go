package chat

import (
	"context"
	"sort"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// Attention tuning.
const (
	// attentionInterval is how often she considers the people who opted in.
	// A timer, unlike everything else she does unprompted, because absence
	// is only noticeable over time — and only for people who asked for it.
	attentionInterval = 10 * time.Minute
	// activityEvery throttles recording that an opted-in person was active
	// elsewhere, so a busy member is one write a minute, not one a message.
	activityEvery = time.Minute
	// engagedWithin is how recently they spoke to her for her to count them
	// as already talking to her.
	engagedWithin = 15 * time.Minute
	// answeredQuickly is how soon after she reached out an answer counts as
	// glad to hear from her, which teaches her more than a late one.
	answeredQuickly = time.Hour
)

// attentionLoop considers, now and then, whether to go after anyone who has
// said she may.
func (s *Service) attentionLoop(ctx context.Context) {
	ticker := time.NewTicker(attentionInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.considerReaching(time.Now())
		}
	}
}

// considerReaching walks every guild she is listening in.
func (s *Service) considerReaching(now time.Time) {
	byGuild := make(map[string][]string)
	for channelID, guildID := range s.store.AllChatChannels() {
		byGuild[guildID] = append(byGuild[guildID], channelID)
	}
	for guildID, channels := range byGuild {
		if s.store.IsChatAttentionOff(guildID) {
			continue
		}
		sort.Strings(channels)
		for _, p := range s.store.AttentionSeekers(guildID) {
			s.maybeReach(guildID, channels, p, now)
		}
	}
}

// maybeReach decides about one person and, if she wants to, queues it.
func (s *Service) maybeReach(guildID string, channels []string, p storage.MindPerson, now time.Time) {
	if !mind.Consented(p.Attention) {
		return
	}
	channelID := reachChannel(p, channels)
	if channelID == "" {
		return
	}

	active := p.LastActiveAt
	if p.LastSeen.After(active) {
		active = p.LastSeen
	}
	closeness, tension, welcome := bondOf(&p).Now(now)
	today := p.ReachToday
	if p.ReachDay != s.day(now) {
		today = 0
	}
	r := mind.Reach{
		Now:        now,
		Longing:    mind.FeelLonging(now, p.LastExchangeAt, active, closeness),
		Closeness:  closeness,
		Tension:    tension,
		Welcome:    welcome,
		Drives:     s.drives(guildID, channelID, now),
		Hour:       now.In(s.loc()).Hour(),
		Today:      today,
		Last:       p.ReachedAt,
		Unanswered: p.Unanswered,
		Engaged:    !p.LastExchangeAt.IsZero() && now.Sub(p.LastExchangeAt) < engagedWithin,
		Jitter:     s.roll(),
	}
	roll := s.roll()
	reach, urge := mind.MayReach(r, roll)
	if !reach {
		return
	}

	// The last one went unanswered, and now she is going again: that is
	// when it counts against how welcome she feels, once per reach.
	if p.Unanswered > 0 {
		s.appraise(guildID, p.UserID, mind.EventReachIgnored, now)
	}

	// Counted when decided, like a volunteered remark: one that then fails
	// to generate still spends the allowance, which errs towards less.
	if err := s.store.MarkReached(guildID, p.UserID, s.day(now), now); err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_reach_record_failed")
		return
	}

	name := p.Username
	why := mind.ReachDirective(name, r)
	if bring, concrete := mind.SomethingToBring(name, factsOf(&p), nil, s.roll()); concrete {
		why += " If it fits: " + bring
	}

	item := mind.Deferred{
		GuildID:      guildID,
		ChannelID:    channelID,
		UserID:       p.UserID,
		Username:     name,
		Trigger:      mind.TriggerReach,
		FormedAt:     now,
		Volunteering: why,
		Journal: s.journalOpen(storage.MindJournal{
			GuildID: guildID, ChannelID: channelID, At: now,
			UserID: p.UserID, Username: name,
			Trigger: string(mind.TriggerReach), Rule: "reached out: she wanted their attention",
			Chance: urge * urge, Roll: roll,
			Mood:     mind.MoodWords(r.Drives),
			Attitude: mind.Attitude(closeness, tension, 0),
			Outcome:  outcomeQueued,
		}),
	}
	select {
	case s.work <- task{item: item}:
		s.log.Info().
			Str("guild_id", guildID).
			Str("channel_id", channelID).
			Str("user_id", p.UserID).
			Float64("urge", urge).
			Msg("chat_reached_out")
	default:
		s.log.Debug().Str("guild_id", guildID).Msg("chat_reach_dropped_busy")
	}
}

// reachChannel is where to find someone: the channel they last talked to her
// in, if she is still listening there, or else the first she listens in.
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

// noteActivity records, for someone who opted in, that they were active in a
// channel she is not listening in: a timestamp only, which is what lets her
// notice being ignored by someone who is plainly around.
func (s *Service) noteActivity(m *discordgo.MessageCreate) {
	p := s.store.GetMindPerson(m.GuildID, m.Author.ID)
	if p == nil || !mind.Consented(p.Attention) {
		return
	}
	now := time.Now()
	if now.Sub(p.LastActiveAt) < activityEvery {
		return
	}
	if err := s.store.ActiveMindPerson(m.GuildID, m.Author.ID, now); err != nil {
		s.log.Debug().Err(err).Str("guild_id", m.GuildID).Msg("chat_activity_record_failed")
	}
}

// noticeExchange records that someone spoke to her, learns from it if she had
// reached out to them, and honours a request to back off.
func (s *Service) noticeExchange(m *discordgo.MessageCreate, person *storage.MindPerson, content string, now time.Time) {
	// Answered after she came to them: the lesson is how welcome she is,
	// and an answer inside the hour teaches more than a late one.
	if person != nil && person.Unanswered > 0 && !person.ReachedAt.IsZero() {
		e := mind.EventReachAnswered
		if now.Sub(person.ReachedAt) < answeredQuickly {
			e = mind.EventReachAnsweredQuickly
		}
		s.appraise(m.GuildID, m.Author.ID, e, now)
	}
	if err := s.store.ExchangeMindPerson(m.GuildID, m.Author.ID, m.ChannelID, now); err != nil {
		s.log.Debug().Err(err).Str("guild_id", m.GuildID).Msg("chat_exchange_record_failed")
	}
	if person == nil || !mind.Consented(person.Attention) || !mind.WantsPeace(content) {
		return
	}
	s.appraise(m.GuildID, m.Author.ID, mind.EventAskedForPeace, now)
	if err := s.store.SetMindConsent(m.GuildID, m.Author.ID, "", now); err != nil {
		s.log.Warn().Err(err).Str("guild_id", m.GuildID).Msg("chat_attention_withdraw_failed")
		return
	}
	s.log.Info().
		Str("guild_id", m.GuildID).
		Str("user_id", m.Author.ID).
		Msg("chat_attention_withdrawn")
}

// loc is the community's timezone.
func (s *Service) loc() *time.Location {
	if s.location == nil {
		return time.UTC
	}
	return s.location
}
