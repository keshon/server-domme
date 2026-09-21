// Package chat runs the persona on Discord: it watches the channels she has
// been let into, hands moments to her mind, and delivers what she decides.
//
// The thinking lives in internal/mind and the remembering in internal/memory;
// this package is the body. It keeps time, keeps the few promises a model
// cannot be trusted with — see Service.overrule — and never lets anything the
// model says reach a channel unchecked. See docs/persona.md.
//
// Generation never runs on a gateway handler goroutine. A backend can take
// most of a minute to answer and COMMAND_TIMEOUT is thirty seconds, so a reply
// built inline would either be killed or would hold a command slot for the
// duration. Observe does only cheap work and hands the rest to workers that
// main owns.
package chat

import (
	"context"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// Service tuning.
const (
	// workers caps how many moments are handled at once across every guild.
	workers = 2
	// queueDepth is how many moments can wait for a worker. A full queue
	// does not drop an answer: it becomes a deferral, the same path a
	// backend failure takes.
	queueDepth = 32
	// defaultGenerateTimeout bounds one moment: the appraisal, the voice
	// and a retry of either.
	defaultGenerateTimeout = 3 * time.Minute
	// retryInterval is how often held answers are reconsidered.
	retryInterval = 15 * time.Second
	// engagedWindow is how recently she must have spoken for the next line
	// from the person she answered to count as carrying on with her.
	engagedWindow = 3 * time.Minute
)

// SessionFunc resolves the current gateway session.
//
// RunSession builds a fresh session on every reconnect, so anything that
// outlives one session has to ask for it per use. A captured pointer goes
// stale and its sends target a closed connection. See docs/architecture.md.
type SessionFunc func() *discordgo.Session

// Deps are what the service needs from the rest of the bot.
type Deps struct {
	Character *mind.Character
	Provider  ai.Provider
	Storage   *storage.Storage
	Memory    *memory.Store
	Session   SessionFunc
	Log       zerolog.Logger
	// Names is every spelling she answers to, most canonical first. What
	// Discord calls her in a guild is added per message.
	Names []string
	// Location is the timezone the community keeps. Nil means UTC.
	Location *time.Location
	// RequestTimeout is how long one backend gets. Zero keeps the default.
	RequestTimeout time.Duration
	// CasualSlips is the odds a message drops the apostrophes from casual
	// contractions; see mind.CasualStyle.
	CasualSlips float64
	// ReflectHour is the hour, in Location, after which she looks back on
	// the day before.
	ReflectHour int
	// Roll supplies randomness. Left nil it uses the global source; a test
	// supplies its own.
	Roll func() float64
	// Now is the clock. Left nil it is time.Now; a test supplies its own.
	Now func() time.Time
}

// task is one moment waiting for a worker.
type task struct {
	item mind.Deferred
	// late marks a second attempt at an answer held back, so it can
	// acknowledge the gap.
	late bool
}

// Service is the running persona.
type Service struct {
	generateTimeout time.Duration

	mind        *mind.Mind
	character   *mind.Character
	names       []string
	store       *storage.Storage
	memory      *memory.Store
	session     SessionFunc
	log         zerolog.Logger
	location    *time.Location
	casualSlips float64
	reflectHour int
	// settleQuiet is how long someone has to stop typing before she reads
	// what they said; see settle.
	settleQuiet time.Duration
	roll        func() float64
	now         func() time.Time

	// replying holds the people she is already handling, per channel, so a
	// burst of lines gets one answer. See Service.answering.
	replyingMu sync.Mutex
	replying   map[string]bool

	// ignored holds, per person per channel, whether she let their last
	// direct approach go. See Service.overrule.
	ignoredMu sync.Mutex
	ignored   map[string]bool

	// overheard is when she last considered a remark not aimed at her, per
	// channel. See Service.overhear.
	overheardMu sync.Mutex
	overheard   map[string]time.Time

	// life is what she started today and when, per guild. See initiative.go.
	lifeMu sync.Mutex
	life   map[string]*lifeState

	// reflected counts attempts to reflect on a day, so a day the model
	// cannot make sense of is not retried forever. See reflect.go.
	reflectMu sync.Mutex
	reflected map[string]int

	conv      *mind.Conversations
	deferrals *mind.Deferrals

	work chan task
}

// New returns a service ready to observe and run.
func New(d Deps) *Service {
	roll := d.Roll
	if roll == nil {
		roll = rand.Float64
	}
	now := d.Now
	if now == nil {
		now = time.Now
	}
	loc := d.Location
	if loc == nil {
		loc = time.UTC
	}

	names := d.Names
	if d.Character != nil {
		names = append([]string{d.Character.Name}, names...)
	}

	// Two backends' worth of deadline per call, and two calls to a moment.
	generateTimeout := defaultGenerateTimeout
	if d.RequestTimeout > 0 && 4*d.RequestTimeout > generateTimeout {
		generateTimeout = 4 * d.RequestTimeout
	}

	return &Service{
		generateTimeout: generateTimeout,

		mind:        &mind.Mind{Character: d.Character, Provider: d.Provider, Memory: d.Memory},
		character:   d.Character,
		names:       mind.CleanNames(names),
		store:       d.Storage,
		memory:      d.Memory,
		session:     d.Session,
		log:         d.Log,
		location:    loc,
		casualSlips: d.CasualSlips,
		reflectHour: d.ReflectHour,
		settleQuiet: settleQuiet,
		roll:        roll,
		now:         now,

		replying:  make(map[string]bool),
		ignored:   make(map[string]bool),
		overheard: make(map[string]time.Time),
		life:      make(map[string]*lifeState),
		reflected: make(map[string]int),
		conv:      mind.NewConversations(),
		deferrals: mind.NewDeferrals(),
		work:      make(chan task, queueDepth),
	}
}

// Run starts the workers and the loops, returning when ctx ends.
func (s *Service) Run(ctx context.Context) {
	var wg sync.WaitGroup
	run := func(f func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f(ctx)
		}()
	}

	for i := 0; i < workers; i++ {
		run(s.workLoop)
	}
	run(s.retryLoop)
	run(s.lifeLoop)
	run(s.reflectLoop)

	s.log.Info().Int("workers", workers).Msg("chat_service_started")
	wg.Wait()
	s.log.Info().Msg("chat_service_stopped")
}

func (s *Service) workLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-s.work:
			s.handle(ctx, t)
			s.doneAnswering(answerKey(t.item.GuildID, t.item.ChannelID, t.item.UserID))
		}
	}
}

// retryLoop re-attempts answers that no backend would produce at the time.
func (s *Service) retryLoop(ctx context.Context) {
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, item := range s.deferrals.Due(s.now()) {
				select {
				case s.work <- task{item: item, late: true}:
				case <-ctx.Done():
					return
				default:
					s.deferrals.Hold(item, s.now())
				}
			}
		}
	}
}

// Observe takes one message from any channel the bot can see.
//
// It runs on the gateway handler goroutine, so everything here is in memory
// or a single storage write. Nothing here talks to a backend.
func (s *Service) Observe(sess *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID == "" || m.Author == nil || m.Author.Bot {
		return
	}
	self := selfID(sess)
	if self != "" && m.Author.ID == self {
		return
	}
	if !s.store.IsChatChannel(m.GuildID, m.ChannelID) {
		// Not a channel she reads. The only thing taken from it is that an
		// opted-in person was around, never what they said.
		s.noteActivity(m)
		return
	}

	now := s.now()
	content := strings.TrimSpace(m.ContentWithMentionsReplaced())
	if content == "" {
		return
	}
	name := displayNameOf(m.Author, m.Member)

	// Before the message is recorded: it asks what the channel looked like
	// just before this arrived.
	followsUp := s.followsUp(m.ChannelID, m.Author.ID, now)

	s.conv.Record(m.ChannelID, mind.Turn{
		UserID:    m.Author.ID,
		Username:  name,
		Content:   content,
		At:        now,
		MessageID: m.ID,
		Mentioned: mentions(m.Message, self),
	})
	if _, err := s.store.SeeMindPerson(m.GuildID, m.Author.ID, name, now); err != nil {
		s.log.Warn().Err(err).Str("guild_id", m.GuildID).Msg("chat_person_record_failed")
	}

	trigger, addressed := s.triggerFor(sess, m, content, followsUp)
	if !addressed {
		if s.overhear(m.GuildID, m.ChannelID, content, now) {
			trigger = mind.TriggerOverheard
		} else {
			return
		}
	} else if err := s.store.ExchangeMindPerson(m.GuildID, m.Author.ID, m.ChannelID, now); err != nil {
		// Speaking to her answers any reach she made; see initiative.go.
		s.log.Debug().Err(err).Str("guild_id", m.GuildID).Msg("chat_exchange_record_failed")
	}

	key := answerKey(m.GuildID, m.ChannelID, m.Author.ID)
	if s.answering(key) {
		// The moment already on its way is built from the conversation as
		// it stands when a worker picks it up, so this line will be in it.
		return
	}

	item := mind.Deferred{
		GuildID:   m.GuildID,
		ChannelID: m.ChannelID,
		MessageID: m.ID,
		UserID:    m.Author.ID,
		Username:  name,
		Content:   content,
		Trigger:   trigger,
		FormedAt:  now,
	}
	select {
	case s.work <- task{item: item}:
		s.startAnswering(key)
	default:
		if mind.Owed(trigger) {
			s.deferrals.Hold(item, now)
		}
		s.log.Debug().Str("guild_id", m.GuildID).Msg("chat_queue_full_deferred")
	}
}

// Overhearing tuning. A remark not aimed at her costs two model calls to
// consider, and the answer is nearly always to stay out of it; these keep
// that from being a call per message in a busy room.
const (
	overhearEvery  = 10 * time.Minute
	overhearChance = 0.3
	overhearWords  = 4
)

// overhear decides whether a remark not aimed at her is worth her
// considering at all. Only in a channel where she may speak up, only now and
// then, and never something too short to have anything in it.
func (s *Service) overhear(guildID, channelID, content string, now time.Time) bool {
	if !s.store.IsChatProactive(guildID, channelID) || len(strings.Fields(content)) < overhearWords {
		return false
	}
	if last := s.lastSpokeAt(channelID); !last.IsZero() && now.Sub(last) < engagedWindow {
		// She is in the middle of something here; a remark between two
		// other people is not hers to take.
		return false
	}
	s.overheardMu.Lock()
	defer s.overheardMu.Unlock()
	if now.Sub(s.overheard[channelID]) < overhearEvery || s.roll() >= overhearChance {
		return false
	}
	s.overheard[channelID] = now
	return true
}

// triggerFor classifies how a message addressed her, if it did.
func (s *Service) triggerFor(sess *discordgo.Session, m *discordgo.MessageCreate, content string, followsUp bool) (mind.Trigger, bool) {
	self := selfID(sess)
	// A reply before a mention: a reply with its ping on also lists her
	// among the mentions, and has to be anchored as a reply.
	if s.repliesToHer(m, self) {
		return mind.TriggerReply, true
	}
	for _, u := range m.Mentions {
		if u.ID == self {
			return mind.TriggerMention, true
		}
	}
	// Her name, spoken to her or about her. Which it was is the appraisal's
	// to read; v1's word list for it is what put her into conversations
	// about her and kept her out of ones addressed to her.
	if mind.SaysName(content, s.namesFor(sess, m.GuildID)) {
		return mind.TriggerNamed, true
	}
	if followsUp {
		return mind.TriggerFollowUp, true
	}
	return "", false
}

// repliesToHer reports whether m is a Discord reply to something she said.
//
// Two ways, because neither is enough alone: ReferencedMessage is documented
// by discordgo as best-effort and replies without it were silently dropped in
// production; MessageReference is always there but carries only an id, which
// is matched against her own recent messages.
func (s *Service) repliesToHer(m *discordgo.MessageCreate, self string) bool {
	if m.ReferencedMessage != nil && m.ReferencedMessage.Author != nil {
		return m.ReferencedMessage.Author.ID == self
	}
	if m.MessageReference == nil || m.MessageReference.MessageID == "" {
		return false
	}
	for _, turn := range s.conv.Recent(m.ChannelID) {
		if turn.FromBot && turn.MessageID == m.MessageReference.MessageID {
			return true
		}
	}
	return false
}

// followsUp reports whether this message carries on an exchange she is in:
// nobody but this person has spoken since she last did, she did so recently,
// and she was talking to them.
//
// "Nobody else since" rather than "she spoke last", because people type in
// bursts; once anyone else has spoken, the thread is not hers to assume.
func (s *Service) followsUp(channelID, userID string, now time.Time) bool {
	turns := s.conv.Recent(channelID)
	i := len(turns) - 1
	for i >= 0 && !turns[i].FromBot && turns[i].UserID == userID {
		i--
	}
	if i < 0 || !turns[i].FromBot || now.Sub(turns[i].At) > engagedWindow {
		return false
	}
	if to := turns[i].To; to != "" {
		return to == userID
	}
	// A turn read back from history does not record who it answered; the
	// last person to speak before her is who she was answering.
	for j := i - 1; j >= 0; j-- {
		if !turns[j].FromBot {
			return turns[j].UserID == userID
		}
	}
	return false
}

// answering reports whether a moment from this person here is already on its
// way to her.
func (s *Service) answering(key string) bool {
	s.replyingMu.Lock()
	defer s.replyingMu.Unlock()
	return s.replying[key]
}

func (s *Service) startAnswering(key string) {
	s.replyingMu.Lock()
	s.replying[key] = true
	s.replyingMu.Unlock()
}

func (s *Service) doneAnswering(key string) {
	s.replyingMu.Lock()
	delete(s.replying, key)
	s.replyingMu.Unlock()
}

// Forget drops what she holds about a channel in memory.
//
// Called when a channel is silenced. Being told to stop reading a channel has
// to take the live conversation with it, or it would still be sent to a
// backend the next time something she started went there.
func (s *Service) Forget(channelID string) {
	if channelID == "" {
		return
	}
	s.conv.Forget(channelID)
	s.deferrals.Drop(channelID)
}

// ForgetGuild moves everything she remembers about a guild aside and clears
// her journal there. It returns where the memory went.
func (s *Service) ForgetGuild(guildID string) (string, error) {
	aside, err := s.memory.Forget(guildID, s.now())
	if err != nil {
		return "", err
	}
	if err := s.store.ForgetMind(guildID); err != nil {
		return aside, err
	}
	return aside, nil
}

// namesFor is every name she answers to in one guild, most canonical first.
// Per guild, because a nickname is set per guild.
func (s *Service) namesFor(sess *discordgo.Session, guildID string) []string {
	return mind.CleanNames(append(discordNames(sess, guildID), s.names...))
}

// discordNames is what Discord calls her here: the guild nickname, the
// display name and the account username.
func discordNames(sess *discordgo.Session, guildID string) []string {
	self := selfID(sess)
	if self == "" {
		return nil
	}
	var names []string
	if guildID != "" {
		if member, err := sess.State.Member(guildID, self); err == nil && member != nil && member.Nick != "" {
			names = append(names, member.Nick)
		}
	}
	if user := sess.State.User; user != nil {
		if user.GlobalName != "" {
			names = append(names, user.GlobalName)
		}
		names = append(names, user.Username)
	}
	return names
}

// DisplayName is the name members see on her messages in a guild.
func (s *Service) DisplayName(sess *discordgo.Session, guildID string) string {
	if names := discordNames(sess, guildID); len(names) > 0 {
		return names[0]
	}
	if len(s.names) > 0 {
		return s.names[0]
	}
	return ""
}

// lastSpokeAt reports when she last said something in a channel.
func (s *Service) lastSpokeAt(channelID string) time.Time {
	turns := s.conv.Recent(channelID)
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].FromBot {
			return turns[i].At
		}
	}
	return time.Time{}
}

// answerKey identifies a person within a channel.
func answerKey(guildID, channelID, userID string) string {
	return guildID + ":" + channelID + ":" + userID
}

func selfID(sess *discordgo.Session) string {
	if sess == nil || sess.State == nil || sess.State.User == nil {
		return ""
	}
	return sess.State.User.ID
}

// displayNameOf resolves the name to show for an author: the guild nickname,
// then the display name, then the account name. Shared with the backfill, so
// the same person never appears under two names in one transcript.
func displayNameOf(author *discordgo.User, member *discordgo.Member) string {
	if member != nil && member.Nick != "" {
		return member.Nick
	}
	if author == nil {
		return ""
	}
	if author.GlobalName != "" {
		return author.GlobalName
	}
	return author.Username
}
