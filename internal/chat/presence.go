package chat

import (
	"context"
	"sort"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/body"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// Presence: she is not always there. The body (internal/body) decides when
// she is online, away or asleep; this is what the service does about it —
// what she sees while she is gone, how she comes back to it, how tired she is
// in what she takes in, and what Discord shows. See docs/persona-v3.md, B.

const (
	// bodyTick is how often the body is lived forward.
	bodyTick = 20 * time.Second
	// bodySaveEvery is how often the body is saved when nothing changed.
	bodySaveEvery = 5 * time.Minute

	// A mention while she is away gets through now and then, like a phone
	// notification: with noticeChance, after a wait between the two bounds.
	noticeChance = 0.35
	noticeMin    = 2 * time.Minute
	noticeMax    = 15 * time.Minute

	// Coming back, she catches up the way someone scrolls back: a first
	// answer after a little while, then one at a time, a pause between.
	catchUpFirstMin = time.Minute
	catchUpFirstMax = 3 * time.Minute
	catchUpGapMin   = 40 * time.Second
	catchUpGapMax   = 90 * time.Second
	// lateAfter is how late an answer has to be before it is framed as
	// late: three minutes is just normal.
	lateAfter = 20 * time.Minute
	// maxMissed bounds what waits for her.
	maxMissed = 32

	// Going to sleep, or away for lack of energy, she says so first about
	// half the time — if she was talking with someone in the last while.
	leaveChance = 0.5
	leaveWithin = 10 * time.Minute

	// talkGap is how long a room can go without her before a conversation
	// there counts as over, for how long she has been talking.
	talkGap = 10 * time.Minute

	// Energy a moment costs her: the effort of talking at all, and more for
	// every other person in it. See body.Body.Drain.
	drainPerMoment = 0.02
	drainPerPerson = 0.01
	// peopleWindow is how far back the transcript is read to count who is in
	// the conversation.
	peopleWindow = 10 * time.Minute

	// Battery thresholds for what she takes in and does. See spec B3.
	batteryFull  = 0.6
	batteryLow   = 0.3
	tailTired    = 10
	tailExhaust  = 4
	recallTired  = 4
	slowWhenLow  = 1.5
	statusOnline = "online"
	statusIdle   = "idle"
)

// catchItem is a missed approach waiting its turn to be answered.
type catchItem struct {
	item mind.Deferred
	due  time.Time
}

// talkState is one room's conversation with her: since when, who with.
type talkState struct {
	since, last time.Time
	people      map[string]bool
}

// adoptBody gives the service its body, restored from the datastore when one
// was saved.
func (s *Service) adoptBody(on bool) {
	if !on {
		return
	}
	now := s.now()
	stored, ok := s.store.ChatBodyState()
	if !ok {
		s.body = body.New(s.location, s.roll, now)
	} else {
		s.body = body.Restore(s.location, s.roll, body.State{
			S: stored.S, B: stored.B, Presence: body.Presence(stored.Presence),
			Since: stored.Since, WokeAt: stored.WokeAt, AwayUntil: stored.AwayUntil,
			Session: stored.Session, Pending: stored.Pending, At: stored.At,
			Woken: stored.Woken, HeldUntil: stored.HeldUntil,
		}, now)
		for _, m := range stored.Missed {
			s.missed = append(s.missed, mind.Deferred{
				GuildID: m.GuildID, ChannelID: m.ChannelID, MessageID: m.MessageID,
				UserID: m.UserID, Username: m.Username, Content: m.Content,
				Trigger: mind.Trigger(m.Trigger), FormedAt: m.At,
			})
		}
	}
	st := s.body.State()
	s.log.Info().Str("presence", string(st.Presence)).Float64("battery", st.B).Msg("chat_body_ready")
}

// online reports whether she is online. Without a body she always is.
func (s *Service) online() bool {
	return s.body == nil || s.body.State().Presence == body.Online
}

// battery is her energy for people; full without a body.
func (s *Service) battery() float64 {
	if s.body == nil {
		return 1
	}
	return s.body.State().B
}

// slowness is how much longer she takes to settle and to type when tired.
func (s *Service) slowness() float64 {
	if s.battery() < batteryFull {
		return slowWhenLow
	}
	return 1
}

// engaged reports whether she has spoken anywhere lately.
func (s *Service) engaged(now time.Time) bool {
	s.bodyMu.Lock()
	defer s.bodyMu.Unlock()
	return !s.spokeAt.IsZero() && now.Sub(s.spokeAt) < engagedWindow
}

func (s *Service) bodyLoop(ctx context.Context) {
	ticker := time.NewTicker(bodyTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.saveBody(s.now())
			return
		case <-ticker.C:
			s.liveBody(ctx)
		}
	}
}

// liveBody lives the body forward and acts on what changed.
func (s *Service) liveBody(ctx context.Context) {
	now := s.now()
	events := s.body.Advance(now, s.engaged(now))

	s.bodyMu.Lock()
	notice := !s.noticeAt.IsZero() && !now.Before(s.noticeAt)
	if notice {
		s.noticeAt = time.Time{}
	}
	s.bodyMu.Unlock()
	if notice {
		events = append(events, s.body.Notice(now)...)
	}

	for _, e := range events {
		s.onPresence(e)
	}
	s.dispatchCatchUp(ctx, now)
	s.applyStatus()

	s.bodyMu.Lock()
	due := len(events) > 0 || now.Sub(s.savedAt) >= bodySaveEvery
	s.bodyMu.Unlock()
	if due {
		s.saveBody(now)
	}
}

// onPresence acts on a change of presence.
func (s *Service) onPresence(e body.Event) {
	st := s.body.State()
	s.log.Info().
		Str("from", string(e.From)).
		Str("to", string(e.To)).
		Str("why", e.Why).
		Float64("battery", st.B).
		Float64("pressure", st.S).
		Msg("chat_presence_changed")

	if e.To == body.Online {
		s.scheduleCatchUp(e.At)
		if (e.Why == body.WhyWoke || e.Why == body.WhyWoken) && s.idleMind {
			// One tick of the idle mind on waking.
			s.idleSoon(e.At)
		}
		return
	}
	if e.From != body.Online {
		return
	}
	// Leaving: what she meant to add a moment later belongs to a moment
	// that is over.
	s.thoughtMu.Lock()
	s.thoughts = nil
	s.thoughtMu.Unlock()
	if e.To == body.Asleep || e.Why == body.WhyTired {
		s.maybeLeave(e.At)
	}
}

// maybeLeave says a word before going, sometimes, in the room she was last
// talking in. The scene carries the fact that she is about to go, stated
// like the time of day; whether and how she says it is hers.
func (s *Service) maybeLeave(now time.Time) {
	s.bodyMu.Lock()
	channel, guild, to, toName, at := s.spokeChannel, s.spokeGuild, s.spokeTo, s.spokeToName, s.spokeAt
	s.bodyMu.Unlock()
	if channel == "" || now.Sub(at) > leaveWithin || s.roll() >= leaveChance {
		return
	}
	item := mind.Deferred{
		GuildID: guild, ChannelID: channel, UserID: to, Username: toName,
		Trigger: mind.TriggerLeave, FormedAt: now,
	}
	select {
	case s.work <- task{item: item}:
	default:
	}
}

// leave says the word before going.
func (s *Service) leave(ctx context.Context, sess *discordgo.Session, t task) {
	genCtx, cancel := context.WithTimeout(ctx, s.generateTimeout)
	defer cancel()
	sc := s.scene(sess, t)
	known, err := s.mind.Know(sc)
	if err != nil {
		s.log.Warn().Err(err).Str("guild_id", sc.GuildID).Msg("chat_memory_read_failed")
	}
	a := mind.Appraisal{Act: mind.ActReply}
	reply, _, err := s.speak(genCtx, sc, known, a, "")
	if err != nil {
		s.log.Info().Err(err).Str("channel_id", sc.ChannelID).Msg("chat_leave_dropped")
		return
	}
	sent, err := s.deliver(genCtx, sess, sc, reply, s.now())
	if err != nil {
		s.log.Warn().Err(err).Str("channel_id", sc.ChannelID).Msg("chat_send_failed")
		return
	}
	if err := s.mind.Said(sc, a, sent.text, "about to go", sent.id); err != nil {
		s.log.Warn().Err(err).Str("guild_id", sc.GuildID).Msg("chat_memory_write_failed")
	}
}

// miss keeps a direct approach that came while she was not online, for when
// she comes back. One per person per channel: the latest is what they are
// waiting on. While she is away — not asleep — a mention sometimes gets
// through.
func (s *Service) miss(item mind.Deferred) {
	now := s.now()
	st := s.body.State()
	s.bodyMu.Lock()
	kept := s.missed[:0]
	for _, m := range s.missed {
		if m.GuildID == item.GuildID && m.ChannelID == item.ChannelID && m.UserID == item.UserID {
			continue
		}
		kept = append(kept, m)
	}
	s.missed = append(kept, item)
	if len(s.missed) > maxMissed {
		s.missed = s.missed[len(s.missed)-maxMissed:]
	}
	if st.Presence == body.Away && mind.Direct(item.Trigger) && s.noticeAt.IsZero() && s.roll() < noticeChance {
		s.noticeAt = now.Add(noticeMin + time.Duration(s.roll()*float64(noticeMax-noticeMin)))
	}
	s.bodyMu.Unlock()

	outcome := outcomeAway
	if st.Presence == body.Asleep {
		outcome = outcomeAsleep
	}
	s.journal(storage.MindJournal{
		GuildID: item.GuildID, ChannelID: item.ChannelID, At: now,
		MessageID: item.MessageID, UserID: item.UserID, Username: item.Username,
		Excerpt: excerpt(item.Content, journalExcerpt), Trigger: string(item.Trigger),
		Outcome: outcome, Reason: "she was not around; she will see it when she is back",
	})
}

// scheduleCatchUp lines up what she missed: a channel at a time, oldest
// first, a pause between answers as if reading back.
func (s *Service) scheduleCatchUp(at time.Time) {
	s.bodyMu.Lock()
	defer s.bodyMu.Unlock()
	if len(s.missed) == 0 {
		return
	}
	missed := s.missed
	s.missed = nil
	first := map[string]time.Time{}
	for _, m := range missed {
		if f, ok := first[m.ChannelID]; !ok || m.FormedAt.Before(f) {
			first[m.ChannelID] = m.FormedAt
		}
	}
	sort.SliceStable(missed, func(i, j int) bool {
		a, b := missed[i], missed[j]
		if a.ChannelID != b.ChannelID {
			return first[a.ChannelID].Before(first[b.ChannelID])
		}
		return a.FormedAt.Before(b.FormedAt)
	})
	due := at.Add(catchUpFirstMin + time.Duration(s.roll()*float64(catchUpFirstMax-catchUpFirstMin)))
	for _, m := range missed {
		s.catchUp = append(s.catchUp, catchItem{item: m, due: due})
		due = due.Add(catchUpGapMin + time.Duration(s.roll()*float64(catchUpGapMax-catchUpGapMin)))
	}
}

// dispatchCatchUp hands the catch-up items that are due to a worker. A fresh
// message meanwhile is handled live, not queued behind them.
func (s *Service) dispatchCatchUp(ctx context.Context, now time.Time) {
	if !s.online() {
		return
	}
	s.bodyMu.Lock()
	defer s.bodyMu.Unlock()
	for len(s.catchUp) > 0 && !s.catchUp[0].due.After(now) {
		c := s.catchUp[0]
		t := task{item: c.item, late: now.Sub(c.item.FormedAt) > lateAfter, catchUp: true}
		select {
		case s.work <- t:
			s.catchUp = s.catchUp[1:]
		case <-ctx.Done():
			return
		default:
			return
		}
	}
}

// noteSpoke records that she said something in a room: for being engaged,
// for the room she would say goodbye in, and for how long she has been
// talking there.
func (s *Service) noteSpoke(sc mind.Scene, now time.Time) {
	s.clearReactions(sc.ChannelID)
	s.bodyMu.Lock()
	defer s.bodyMu.Unlock()
	s.spokeAt, s.spokeGuild, s.spokeChannel = now, sc.GuildID, sc.ChannelID
	if sc.UserID != "" {
		s.spokeTo, s.spokeToName = sc.UserID, sc.Username
	}
	t := s.talk[sc.ChannelID]
	if t == nil || now.Sub(t.last) > talkGap {
		t = &talkState{since: now, people: map[string]bool{}}
		s.talk[sc.ChannelID] = t
	}
	t.last = now
	if sc.UserID != "" {
		t.people[sc.UserID] = true
	}
}

// bodyScene adds what the body means for a moment to its scene: the facts
// she is given about it, and how much she takes in.
func (s *Service) bodyScene(sc *mind.Scene, now time.Time) {
	if s.body == nil {
		return
	}
	st := s.body.State()
	if st.Presence != body.Asleep {
		sc.Woke, sc.WokenEarly = st.WokeAt, st.Woken
	}
	s.bodyMu.Lock()
	if t := s.talk[sc.ChannelID]; t != nil && now.Sub(t.last) <= talkGap {
		sc.TalkingFor, sc.TalkingWith = now.Sub(t.since), len(t.people)
	}
	s.bodyMu.Unlock()

	switch {
	case st.B < batteryLow:
		sc.Turns = lastTurns(sc.Turns, tailExhaust)
		sc.RecallCap, sc.ShortExamples = -1, true
	case st.B < batteryFull:
		sc.Turns = lastTurns(sc.Turns, tailTired)
		sc.RecallCap = recallTired
	}
}

func lastTurns(turns []mind.Turn, n int) []mind.Turn {
	if len(turns) > n {
		return turns[len(turns)-n:]
	}
	return turns
}

// drain takes what a moment cost her.
func (s *Service) drain(sc mind.Scene) {
	if s.body == nil {
		return
	}
	people := map[string]bool{}
	for _, t := range sc.Turns {
		if !t.FromBot && t.UserID != "" && sc.Now.Sub(t.At) <= peopleWindow {
			people[t.UserID] = true
		}
	}
	cost := drainPerMoment
	if len(people) > 1 {
		cost += drainPerPerson * float64(len(people)-1)
	}
	s.body.Drain(cost)
}

// applyStatus shows her presence in Discord: online when she is, idle when
// she is away or asleep, and a line under her name saying what she is doing.
// Never invisible — the bot's other commands keep working, and an offline
// bot reads as a broken one. Sent again after a reconnect, which resets it,
// and otherwise only when something changed: Discord limits how often a
// status may change.
func (s *Service) applyStatus() {
	sess := s.session()
	if sess == nil {
		return
	}
	status := statusIdle
	if s.online() {
		status = statusOnline
	}
	text := s.statusText(s.now())
	s.bodyMu.Lock()
	same := s.statusSent == status+"\x00"+text && s.statusSess == sess
	s.bodyMu.Unlock()
	if same {
		return
	}
	update := discordgo.UpdateStatusData{Status: status}
	if text != "" {
		update.Activities = []*discordgo.Activity{{Name: "Custom Status", Type: discordgo.ActivityTypeCustom, State: text}}
	}
	if err := sess.UpdateStatusComplex(update); err != nil {
		s.log.Debug().Err(err).Msg("chat_status_failed")
		return
	}
	s.bodyMu.Lock()
	s.statusSent, s.statusSess = status+"\x00"+text, sess
	s.bodyMu.Unlock()
}

// Status lines, under her name in the member list. Fixed words, from facts
// of her body and her talking — never a channel, a server or a person: the
// status is one for the whole bot, and shows in every server she is in.
const (
	statusAsleep  = "💤 asleep"
	statusWoken   = "woken up. not thrilled"
	statusJustUp  = "just woke up"
	statusAway    = "away for a bit"
	statusRecharg = "recharging"
	statusChat    = "chatting"
	statusAround  = "around"
)

// Within these, waking is still news.
const (
	justUpFor = 30 * time.Minute
	wokenFor  = time.Hour
)

// statusText is the line under her name.
func (s *Service) statusText(now time.Time) string {
	if s.body != nil {
		st := s.body.State()
		switch st.Presence {
		case body.Asleep:
			return statusAsleep
		case body.Away:
			if st.AwayUntil.IsZero() {
				return statusRecharg
			}
			return statusAway
		}
		if st.Woken && now.Sub(st.WokeAt) < wokenFor {
			return statusWoken
		}
		if !st.WokeAt.IsZero() && now.Sub(st.WokeAt) < justUpFor {
			return statusJustUp
		}
	}
	if s.engaged(now) {
		return statusChat
	}
	return statusAround
}

// statusLoop keeps the status line current: from "chatting" back to
// "around" when she stops, and waking news going stale.
func (s *Service) statusLoop(ctx context.Context) {
	ticker := time.NewTicker(statusEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.applyStatus()
		}
	}
}

// statusEvery is how often the status line is looked at.
const statusEvery = time.Minute

// WakeOutcome is what came of waking her.
type WakeOutcome string

// Outcomes of Wake.
const (
	WakeNoBody  WakeOutcome = "no-body"
	WakeAwake   WakeOutcome = "awake"
	WakeFromBed WakeOutcome = "asleep"
	WakeBack    WakeOutcome = "away"
)

// Wake wakes her, for /chat wake: from sleep she is woken early and kept up
// a while, with her tiredness where it was; from away she comes back. What
// she missed is caught up on as on any return. One body for every server.
func (s *Service) Wake() WakeOutcome {
	if s.body == nil {
		return WakeNoBody
	}
	now := s.now()
	was := s.body.State().Presence
	events := s.body.Wake(now)
	if len(events) == 0 {
		return WakeAwake
	}
	for _, e := range events {
		s.onPresence(e)
	}
	s.applyStatus()
	s.saveBody(now)
	if was == body.Asleep {
		return WakeFromBed
	}
	return WakeBack
}

// saveBody writes the body and what she missed to the datastore.
func (s *Service) saveBody(now time.Time) {
	if s.body == nil {
		return
	}
	st := s.body.State()
	s.bodyMu.Lock()
	missed := make([]storage.MissedApproach, 0, len(s.missed)+len(s.catchUp))
	add := func(m mind.Deferred) {
		missed = append(missed, storage.MissedApproach{
			GuildID: m.GuildID, ChannelID: m.ChannelID, MessageID: m.MessageID,
			UserID: m.UserID, Username: m.Username, Content: m.Content,
			Trigger: string(m.Trigger), At: m.FormedAt,
		})
	}
	for _, m := range s.missed {
		add(m)
	}
	for _, c := range s.catchUp {
		add(c.item)
	}
	s.savedAt = now
	s.bodyMu.Unlock()
	err := s.store.SetChatBodyState(storage.ChatBody{
		S: st.S, B: st.B, Presence: string(st.Presence), Since: st.Since, WokeAt: st.WokeAt,
		AwayUntil: st.AwayUntil, Session: st.Session, Pending: st.Pending, At: st.At, Missed: missed,
		Woken: st.Woken, HeldUntil: st.HeldUntil,
	})
	if err != nil {
		s.log.Warn().Err(err).Msg("chat_body_save_failed")
	}
}

// BodyStatus is her body for /chat status.
type BodyStatus struct {
	Presence string
	Since    time.Time
	WokeAt   time.Time
	Battery  float64
	Missed   int
}

// Body reports her body, and false when she has none.
func (s *Service) Body() (BodyStatus, bool) {
	if s.body == nil {
		return BodyStatus{}, false
	}
	st := s.body.State()
	s.bodyMu.Lock()
	missed := len(s.missed) + len(s.catchUp)
	s.bodyMu.Unlock()
	return BodyStatus{
		Presence: string(st.Presence), Since: st.Since.In(s.location), WokeAt: st.WokeAt.In(s.location),
		Battery: st.B, Missed: missed,
	}, true
}
