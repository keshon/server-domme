// Package chat runs the conversational persona: it watches opted-in channels,
// decides whether the character has anything to say, and speaks through a
// backend pool when she does.
//
// The cognition lives in internal/mind and knows nothing about Discord or
// storage; this package is the wiring. That split is what lets every decision
// about when to speak be tested without a gateway.
//
// Generation never runs on a gateway handler goroutine. A free relay can take
// most of a minute to answer and COMMAND_TIMEOUT is thirty seconds, so a reply
// built inline would either be killed or would hold a command slot for the
// duration. Observe therefore does only the cheap deterministic work and hands
// the rest to workers that main owns.
package chat

import (
	"context"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// Service tuning.
const (
	// workers caps how many replies are generated at once across every guild.
	// The free relays are the bottleneck rather than this process, and a burst
	// of concurrent requests is the fastest way to be rate limited by all of
	// them at once.
	workers = 2
	// queueDepth is how many approaches can wait for a worker. A full queue
	// does not drop the approach: it becomes a deferral, the same path a
	// backend failure takes.
	queueDepth = 32
	// generateTimeout bounds one reply attempt including failover across
	// backends.
	generateTimeout = 90 * time.Second
	// retryInterval is how often held approaches are reconsidered.
	retryInterval = 15 * time.Second
)

// SessionFunc resolves the current gateway session.
//
// RunSession builds a fresh session on every reconnect, so the retry loop —
// which outlives any one session — has to ask for it per use. A captured
// pointer goes stale and its sends target a closed connection. See
// docs/architecture.md.
type SessionFunc func() *discordgo.Session

// Deps are what the service needs from the rest of the bot.
type Deps struct {
	Character *mind.Character
	Provider  ai.Provider
	Storage   *storage.Storage
	Session   SessionFunc
	Log       zerolog.Logger
	// Names is every spelling she answers to, most canonical first. Whatever
	// Discord reports for her in a given guild is added to it per message.
	Names []string
	// Attention overrides how readily she answers. The zero value takes the
	// defaults.
	Attention mind.Attention
	// Roll supplies randomness for the speak-or-stay-quiet decision. Left nil
	// it uses the global source; a test supplies its own.
	Roll func() float64
}

// task is one approach waiting to be answered.
type task struct {
	item mind.Deferred
	// late marks a second attempt at something held back, so the reply can
	// acknowledge the gap.
	late bool
}

// Service is the running persona.
type Service struct {
	character *mind.Character
	names     []string
	provider  ai.Provider
	store     *storage.Storage
	session   SessionFunc
	log       zerolog.Logger
	attention mind.Attention
	budget    mind.Budget
	roll      func() float64

	conv       *mind.Conversations
	deferrals  *mind.Deferrals
	encounters *mind.Encounters

	work chan task
}

// New returns a service ready to observe and run.
func New(d Deps) *Service {
	attention := d.Attention
	if attention.MentionChance == 0 && attention.NamedChance == 0 && attention.ReplyChance == 0 {
		attention = mind.DefaultAttention()
	}
	roll := d.Roll
	if roll == nil {
		roll = rand.Float64
	}

	names := d.Names
	if d.Character != nil {
		names = append([]string{d.Character.Name}, names...)
	}

	return &Service{
		character:  d.Character,
		names:      mind.CleanNames(names),
		provider:   d.Provider,
		store:      d.Storage,
		session:    d.Session,
		log:        d.Log,
		attention:  attention,
		budget:     mind.DefaultBudget(),
		roll:       roll,
		conv:       mind.NewConversations(),
		deferrals:  mind.NewDeferrals(),
		encounters: mind.NewEncounters(),
		work:       make(chan task, queueDepth),
	}
}

// Run starts the workers and the deferral retry loop, returning when ctx ends.
func (s *Service) Run(ctx context.Context) {
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.workLoop(ctx)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.retryLoop(ctx)
	}()

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
			s.speak(ctx, t)
		}
	}
}

// retryLoop re-attempts approaches that no backend would answer at the time.
func (s *Service) retryLoop(ctx context.Context) {
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, item := range s.deferrals.Due(time.Now()) {
				select {
				case s.work <- task{item: item, late: true}:
				case <-ctx.Done():
					return
				default:
					// Workers are busy. Put it back rather than lose it.
					s.deferrals.Hold(item, time.Now())
				}
			}
		}
	}
}

// Observe takes one message from a watched channel.
//
// It runs on the gateway handler goroutine, so everything here is either in
// memory or a single storage write. Nothing in this function talks to a
// backend.
func (s *Service) Observe(sess *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID == "" || m.Author == nil {
		return
	}
	// Never converse with another bot. Two personas in one channel answer each
	// other forever, and every turn of it costs a backend request.
	if m.Author.Bot {
		return
	}
	if selfID(sess) != "" && m.Author.ID == selfID(sess) {
		return
	}
	if !s.store.IsChatChannel(m.GuildID, m.ChannelID) {
		return
	}

	now := time.Now()
	content := strings.TrimSpace(m.ContentWithMentionsReplaced())
	if content == "" {
		return
	}

	name := displayName(m)

	// Computed before the message is recorded, because it asks what the
	// channel looked like just before it arrived.
	followsUp := s.followsUp(m.ChannelID, m.Author.ID, now)

	s.conv.Record(m.ChannelID, mind.Turn{
		UserID:    m.Author.ID,
		Username:  name,
		Content:   content,
		At:        now,
		MessageID: m.ID,
	})

	if _, err := s.store.SeeMindPerson(m.GuildID, m.Author.ID, name, now); err != nil {
		s.log.Warn().Err(err).Str("guild_id", m.GuildID).Msg("chat_person_record_failed")
	}

	trigger, addressed := s.triggerFor(sess, m, content, followsUp)
	if !addressed {
		return
	}

	key := encounterKey(m.GuildID, m.ChannelID, m.Author.ID)
	first, ignoredLast := s.encounters.Approach(key)

	outcome := mind.Decide(s.attention, mind.Situation{
		Trigger:       trigger,
		Now:           now,
		FirstApproach: first,
		IgnoredLast:   ignoredLast,
		LastSpokeAt:   s.lastSpokeAt(m.ChannelID),
	}, s.roll())
	s.encounters.Record(key, outcome)

	if outcome == mind.OutcomeIgnore {
		// Logged at info rather than debug: from outside, a deliberate silence
		// and a broken bot look identical, and this line is the only way to
		// tell them apart afterwards.
		s.log.Info().
			Str("guild_id", m.GuildID).
			Str("channel_id", m.ChannelID).
			Str("trigger", string(trigger)).
			Msg("chat_approach_ignored")
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
	default:
		// Everything is busy. Holding it takes the same path a failed backend
		// does, so a busy moment produces a late answer rather than none.
		s.deferrals.Hold(item, now)
		s.log.Debug().Str("guild_id", m.GuildID).Msg("chat_queue_full_deferred")
	}
}

// triggerFor classifies how a message addressed the character, if it did.
func (s *Service) triggerFor(sess *discordgo.Session, m *discordgo.MessageCreate, content string, followsUp bool) (mind.Trigger, bool) {
	self := selfID(sess)

	for _, u := range m.Mentions {
		if u.ID == self {
			return mind.TriggerMention, true
		}
	}

	if s.repliesToHer(m, self) {
		return mind.TriggerReply, true
	}

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
// Two ways, because neither is sufficient alone. ReferencedMessage carries the
// author but discordgo documents it as best-effort — "the backend did not
// attempt to fetch the message that was being replied to" — and a reply whose
// target it omitted was being silently ignored in production. MessageReference
// is always present on a reply but carries only an id, so it is matched
// against the ids of her own recent messages, which is why sent messages
// record theirs. See mind.Turn.MessageID.
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

// followsUp reports whether this message continues an exchange she is already
// in: she spoke last in the channel, recently, and to this same person.
//
// All three conditions carry weight. "She spoke last" is what keeps her out of
// a conversation between two other people — once someone else has spoken, the
// thread is no longer hers to assume. "Same person" stops her fielding a
// bystander's unrelated remark. The window stops a reply arriving against a
// conversation everyone has left.
func (s *Service) followsUp(channelID, userID string, now time.Time) bool {
	turns := s.conv.Recent(channelID)
	if len(turns) == 0 {
		return false
	}

	last := turns[len(turns)-1]
	if !last.FromBot || now.Sub(last.At) > s.attention.EngagedWindow {
		return false
	}

	// Whoever she was answering is the last person to speak before her.
	for i := len(turns) - 2; i >= 0; i-- {
		if turns[i].FromBot {
			continue
		}
		return turns[i].UserID == userID
	}
	return false
}

// namesFor is every name she answers to in one guild, most canonical first.
//
// Resolved per guild rather than once at startup because two of the three
// sources are per guild: the nickname is set on the member, and the account
// name can be changed under a running process. Discord's names come first —
// they are what members actually see and what a mention expands to, so they
// are the identity, and CHAT_NAME is an alias for it rather than the other way
// round.
func (s *Service) namesFor(sess *discordgo.Session, guildID string) []string {
	return mind.CleanNames(append(s.discordNames(sess, guildID), s.names...))
}

// discordNames reports what Discord calls her here: the guild nickname, the
// display name and the account username, in the order a member is most likely
// to use. Missing state yields fewer names rather than an error — a cache miss
// costs a name-drop she does not notice, not a broken reply.
func (s *Service) discordNames(sess *discordgo.Session, guildID string) []string {
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

// DisplayName is the name members see on her messages in a guild: the nickname
// when one is set, otherwise the account name.
func (s *Service) DisplayName(sess *discordgo.Session, guildID string) string {
	if names := s.discordNames(sess, guildID); len(names) > 0 {
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

// encounterKey identifies a person within a channel, so being ignored in one
// place does not force a reply in another.
func encounterKey(guildID, channelID, userID string) string {
	return guildID + ":" + channelID + ":" + userID
}

func selfID(sess *discordgo.Session) string {
	if sess == nil || sess.State == nil || sess.State.User == nil {
		return ""
	}
	return sess.State.User.ID
}

// displayName prefers the per-guild nickname, which is what everyone else in
// the channel sees and therefore what she should call them.
func displayName(m *discordgo.MessageCreate) string {
	if m.Member != nil && m.Member.Nick != "" {
		return m.Member.Nick
	}
	if m.Author == nil {
		return ""
	}
	if m.Author.GlobalName != "" {
		return m.Author.GlobalName
	}
	return m.Author.Username
}
