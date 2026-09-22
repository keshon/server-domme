package chat

import (
	"context"
	"sort"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/body"
	"github.com/keshon/server-domme/internal/memory"
	"github.com/keshon/server-domme/internal/mind"
)

// Inner life, as the service runs it: the idle mind's ticks, walks through
// channels she reads without speaking in, and impulses turned into things
// she starts when the rails allow. See docs/persona-v3.md, E, F3 and H1.

const (
	// idleCheck is how often the service looks whether a tick is due.
	idleCheck = time.Minute
	// A tick comes every idleMin to idleMax, varied, while she is awake.
	idleMin = 45 * time.Minute
	idleMax = 90 * time.Minute
	// walkChance is the odds a tick is a walk, when there is somewhere with
	// something new to walk through.
	walkChance = 0.5
	// walkLines is the most of a channel a walk takes in.
	walkLines = 30
	// hereWithin is how recently someone must have spoken in a room she
	// answers in to count as around; seenWithin how recently at all for
	// her to think of going to them.
	hereWithin = 30 * time.Minute
	seenWithin = 7 * 24 * time.Hour
	// maxCandidates is how many people the idle mind is offered.
	maxCandidates = 8
)

func (s *Service) idleLoop(ctx context.Context) {
	ticker := time.NewTicker(idleCheck)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.idleDue(ctx)
		}
	}
}

// idleGuilds are the guilds she has anywhere to be in.
func (s *Service) idleGuilds() []string {
	seen := map[string]bool{}
	for _, g := range s.store.AllChatChannels() {
		seen[g] = true
	}
	out := make([]string, 0, len(seen))
	for g := range seen {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// idleDue runs the ticks that are due.
func (s *Service) idleDue(ctx context.Context) {
	now := s.now()
	if !s.awake(now.In(s.location)) {
		return
	}
	for _, guildID := range s.idleGuilds() {
		if ctx.Err() != nil {
			return
		}
		s.idleMu.Lock()
		next, ok := s.nextIdle[guildID]
		if !ok {
			// First look after a start: not all at once.
			next = now.Add(time.Duration(s.roll() * float64(idleMin)))
			s.nextIdle[guildID] = next
		}
		due := !now.Before(next)
		if due {
			s.nextIdle[guildID] = now.Add(idleMin + time.Duration(s.roll()*float64(idleMax-idleMin)))
		}
		s.idleMu.Unlock()
		if due {
			s.idleTick(ctx, guildID, now)
		}
	}
}

// awake reports whether she is awake to think: with a body, not asleep —
// away counts, she is just not looking at the chat; without one, outside the
// quiet hours.
func (s *Service) awake(local time.Time) bool {
	if s.body != nil {
		return s.body.State().Presence != body.Asleep
	}
	h := local.Hour()
	return h < quietFrom && h >= quietUntil
}

// idleSoon brings every guild's next tick forward to now: on waking.
func (s *Service) idleSoon(now time.Time) {
	s.idleMu.Lock()
	defer s.idleMu.Unlock()
	for _, g := range s.idleGuilds() {
		s.nextIdle[g] = now
	}
}

// idleTick is one tick of the idle mind in a guild.
func (s *Service) idleTick(ctx context.Context, guildID string, now time.Time) {
	sess := s.session()
	local := now.In(s.location)
	in := mind.Idle{GuildID: guildID, Now: local, QuietFor: s.quietFor(guildID, now)}
	if sess != nil && sess.State != nil {
		if g, err := sess.State.Guild(guildID); err == nil && g != nil {
			in.GuildName = g.Name
		}
	}
	channels := s.store.GetChatChannels(guildID)
	in.Rooms = firstProactive(s.store, guildID, channels) != ""
	in.People = s.candidates(guildID, channels, now)
	if threads, err := s.memory.Threads(guildID); err == nil {
		for _, t := range memory.Unfinished(threads) {
			if t.Due.IsZero() || !t.Due.After(now) {
				in.Due = append(in.Due, t)
			}
		}
	}
	var walkedChannel string
	if s.walks && s.roll() < walkChance {
		if w, channelID := s.walk(sess, guildID); w != nil {
			in.Walk, walkedChannel = w, channelID
		}
	}

	genCtx, cancel := context.WithTimeout(ctx, s.generateTimeout)
	defer cancel()
	res, err := s.mind.IdleThink(genCtx, in)
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_idle_failed")
		return
	}
	if walkedChannel != "" {
		s.idleMu.Lock()
		s.walked[walkedChannel] = now
		s.idleMu.Unlock()
	}
	s.log.Info().
		Str("guild_id", guildID).
		Bool("walk", in.Walk != nil).
		Bool("caught", res.Caught != "").
		Bool("impulse", res.Impulse != nil).
		Str("backend", res.Backend).
		Msg("chat_idle_ticked")

	if res.Impulse != nil && s.impulses {
		s.act(genCtx, sess, guildID, local, in, *res.Impulse)
	}
}

// candidates are the people an impulse may be aimed at: those seen lately,
// most recent first, and whether each is around right now.
func (s *Service) candidates(guildID string, channels []string, now time.Time) []mind.Candidate {
	people := s.store.MindPeople(guildID)
	sort.Slice(people, func(i, j int) bool { return people[i].LastSeen.After(people[j].LastSeen) })
	var out []mind.Candidate
	for _, p := range people {
		if p.Username == "" || now.Sub(p.LastSeen) > seenWithin {
			continue
		}
		_, here := s.whereIs(p.UserID, channels, now)
		out = append(out, mind.Candidate{ID: p.UserID, Name: p.Username, Here: here})
		if len(out) == maxCandidates {
			break
		}
	}
	return out
}

// whereIs is the room she answers in that someone spoke in most recently,
// within hereWithin.
func (s *Service) whereIs(userID string, channels []string, now time.Time) (string, bool) {
	var best string
	var at time.Time
	for _, c := range channels {
		turns := s.conv.Recent(c)
		for i := len(turns) - 1; i >= 0; i-- {
			if turns[i].UserID == userID && !turns[i].FromBot {
				if now.Sub(turns[i].At) <= hereWithin && turns[i].At.After(at) {
					best, at = c, turns[i].At
				}
				break
			}
		}
	}
	return best, best != ""
}

// walk picks a channel she reads without speaking in that has something new
// since her last pass, weighted towards the busier, and the lines since.
func (s *Service) walk(sess *discordgo.Session, guildID string) (*mind.Walk, string) {
	type option struct {
		channelID string
		lines     []mind.Turn
	}
	var options []option
	total := 0
	for _, c := range s.store.GetChatReads(guildID) {
		if AgeRestricted(sess, c) {
			continue
		}
		s.idleMu.Lock()
		last := s.walked[c]
		s.idleMu.Unlock()
		var fresh []mind.Turn
		for _, t := range s.conv.Recent(c) {
			if t.At.After(last) && !t.FromBot {
				fresh = append(fresh, t)
			}
		}
		if len(fresh) == 0 {
			continue
		}
		if len(fresh) > walkLines {
			fresh = fresh[len(fresh)-walkLines:]
		}
		options = append(options, option{c, fresh})
		total += len(fresh)
	}
	if total == 0 {
		return nil, ""
	}
	pick := s.roll() * float64(total)
	for _, o := range options {
		pick -= float64(len(o.lines))
		if pick < 0 || o.channelID == options[len(options)-1].channelID {
			name := channelNames(sess, []string{o.channelID})[o.channelID]
			return &mind.Walk{Channel: name, Lines: o.lines}, o.channelID
		}
	}
	return nil, ""
}

// act turns an impulse into something she starts, if the rails allow: she
// is awake to it and within the day's limits; to someone, only where they
// are around or where they agreed to be reached; to a room, only one she
// may speak up in.
func (s *Service) act(ctx context.Context, sess *discordgo.Session, guildID string, local time.Time, in mind.Idle, imp mind.Impulse) {
	if sess == nil || !s.awakeToStart(local) || !s.mayStart(guildID, local) {
		s.log.Info().Str("guild_id", guildID).Msg("chat_impulse_held_back")
		return
	}
	channels := s.store.GetChatChannels(guildID)
	names := channelNames(sess, channels)
	var o mind.Opening
	switch {
	case imp.Person == nil:
		c := firstProactive(s.store, guildID, channels)
		if c == "" {
			return
		}
		if s.talkingIn(c, s.now()) {
			s.fold(guildID, local, imp, "a conversation is going on there")
			return
		}
		o = mind.Opening{Trigger: mind.TriggerStart, ChannelID: c, ChannelName: names[c], Turns: s.conv.Recent(c)}
	default:
		p := imp.Person
		if why := s.engagedWith(guildID, p.ID, s.now()); why != "" {
			s.fold(guildID, local, imp, why)
			return
		}
		if c, here := s.whereIs(p.ID, channels, s.now()); here {
			// They are around: she is joining a room they are in, as with
			// a follow-up on sight. No tag needed, no consent asked.
			o = mind.Opening{Trigger: mind.TriggerStart, ChannelID: c, ChannelName: names[c], UserID: p.ID, Username: p.Name, Turns: s.conv.Recent(c)}
			break
		}
		mp := s.store.GetMindPerson(guildID, p.ID)
		if mp == nil || !Consented(mp.Attention) || s.store.IsChatAttentionOff(guildID) || !s.mayReach(*mp, s.now()) {
			s.log.Info().Str("guild_id", guildID).Msg("chat_impulse_not_reachable")
			return
		}
		c := reachChannel(*mp, channels)
		o = mind.Opening{Trigger: mind.TriggerReach, ChannelID: c, ChannelName: names[c], UserID: p.ID, Username: p.Name, Turns: s.conv.Recent(c)}
	}
	if imp.Person != nil {
		for i := range in.Due {
			if in.Due[i].Person.ID == imp.Person.ID {
				th := in.Due[i]
				o.Thread = &th
				break
			}
		}
	}
	base := mind.Scene{
		GuildID: guildID, GuildName: in.GuildName, Brief: s.store.GetChatBrief(guildID),
		SelfName: s.DisplayName(sess, guildID), Now: local, QuietFor: in.QuietFor,
	}
	// The reason is what the impulse came from, so the voice has the
	// substance and not only the gist: told only "the thing she has been
	// sitting on", a voice makes the thing up.
	why := imp.About
	if imp.From != "" {
		why += " — it comes from: " + imp.From
	}
	s.start(ctx, sess, base, o, mind.Plan{Why: why, Intent: imp.About})
}

// engagedWith reports why she should not start something with someone now,
// or "": they are in an exchange with her already, or waiting on an answer
// she owes them. Starting something then greets them twice — in production
// she answered someone and, the same minute, opened on them again.
func (s *Service) engagedWith(guildID, userID string, now time.Time) string {
	if p := s.store.GetMindPerson(guildID, userID); p != nil && now.Sub(p.LastExchangeAt) < activeWithin {
		return "they are talking with her already"
	}
	s.bodyMu.Lock()
	defer s.bodyMu.Unlock()
	for _, m := range s.missed {
		if m.GuildID == guildID && m.UserID == userID {
			return "she owes them an answer"
		}
	}
	for _, c := range s.catchUp {
		if c.item.GuildID == guildID && c.item.UserID == userID {
			return "she owes them an answer"
		}
	}
	return ""
}

// talkingIn reports whether she is in a conversation in a room: she spoke
// there within activeWithin.
func (s *Service) talkingIn(channelID string, now time.Time) bool {
	s.bodyMu.Lock()
	defer s.bodyMu.Unlock()
	t := s.talk[channelID]
	return t != nil && now.Sub(t.last) < activeWithin
}

// fold keeps an impulse she will not act on by starting something as what
// is on her mind, where the conversation she is in can take it up — the
// way a person brings a thing up in a conversation already going instead
// of opening a new one.
func (s *Service) fold(guildID string, local time.Time, imp mind.Impulse, why string) {
	err := s.memory.UpdateSelf(guildID, func(me *memory.Self) {
		me.OnMind, me.OnMindAt = imp.About, local
	})
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", guildID).Msg("chat_memory_write_failed")
		return
	}
	s.log.Info().Str("guild_id", guildID).Str("reason", why).Msg("chat_impulse_folded")
}

// reacted counts a reaction she started toward the day's limit: a third of
// something said.
func (s *Service) reacted(guildID string, now time.Time) {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	day := now.Format("2006-01-02")
	st := s.life[guildID]
	if st == nil || st.day != day {
		st = &lifeState{day: day}
		s.life[guildID] = st
	}
	st.count++
}
