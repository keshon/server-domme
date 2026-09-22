package chat

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
)

// voiceSession is how long her voice keeps the backend it last spoke
// through. Every model has a native voice, so a conversation that failed over
// to another backend stays on it rather than changing voice again the moment
// the preferred one recovers.
//
// With a body, a session is a stretch online: one voice for as long as she is
// online, reset when she goes away or to sleep, so a change of voice happens
// while she is gone. Without one, a session is the engaged window: she has
// spoken somewhere within it. Bot-wide, not per channel, so she never speaks
// in two voices in two channels at once.
const voiceSession = engagedWindow

// speak is mind.Speak through the voice she has been using.
func (s *Service) speak(ctx context.Context, sc mind.Scene, k mind.Known, a mind.Appraisal, why string) (string, string, error) {
	now := s.now()
	session := 0
	if s.body != nil {
		session = s.body.State().Session
	}
	s.voiceMu.Lock()
	switch {
	case s.body != nil && s.voiceSession == session:
		ctx = ai.WithPrefer(ctx, s.voiceName)
	case s.body == nil && now.Sub(s.voiceAt) < voiceSession:
		ctx = ai.WithPrefer(ctx, s.voiceName)
	}
	s.voiceMu.Unlock()

	reply, backend, err := s.mind.Speak(ctx, sc, k, a, why)
	if err == nil && backend != "" {
		s.voiceMu.Lock()
		s.voiceName, s.voiceAt, s.voiceSession = backend, s.now(), session
		s.voiceMu.Unlock()
	}
	return reply, backend, err
}

// adoptPool takes the pool the service speaks through, remembers its
// arrangement as the environment gave it, and restores the one the owner set
// with /chat backends, if any.
func (s *Service) adoptPool(p *ai.Pool) {
	if p == nil {
		return
	}
	s.pool = p
	s.poolDefaults = p.Settings()
	if s.store == nil {
		return
	}
	stored, ok := s.store.ChatBackendArrangement()
	if !ok {
		return
	}
	mode, _ := ai.ParseMode(stored.Mode)
	unknown := p.Apply(ai.Settings{Mode: mode, Order: stored.Order, Voice: stored.Voice, Off: stored.Off})
	for _, name := range unknown {
		// A backend renamed or removed from CHAT_BACKENDS since the owner
		// arranged them. Not fatal: the rest of the arrangement holds.
		s.log.Warn().Str("backend", name).Msg("chat_stored_backend_unknown")
	}
	s.log.Info().
		Strs("order", p.Settings().Order).
		Strs("voice", p.Settings().Voice).
		Strs("off", p.Settings().Off).
		Msg("chat_backends_restored")
}

// ErrNoPool is a backend change asked of a service that has no pool.
var ErrNoPool = errors.New("chat: the backends are not a pool that can be arranged")

// Backends is how the backends are arranged and how each is doing.
type Backends struct {
	Mode  ai.Mode
	Stats []ai.BackendStat
	// Stored is whether the arrangement is the owner's rather than the
	// environment's.
	Stored bool
}

// Backends reports the backends, or false when there is no pool.
func (s *Service) Backends() (Backends, bool) {
	if s.pool == nil {
		return Backends{}, false
	}
	_, stored := s.store.ChatBackendArrangement()
	return Backends{Mode: s.pool.Mode(), Stats: s.pool.Stats(), Stored: stored}, true
}

// ArrangeBackends applies a change to the pool and stores the result, so it
// survives a restart. A change that fails leaves the pool as it was.
func (s *Service) ArrangeBackends(change func(*ai.Pool) error) error {
	if s.pool == nil {
		return ErrNoPool
	}
	before := s.pool.Settings()
	if err := change(s.pool); err != nil {
		s.restore(before)
		return err
	}
	after := s.pool.Settings()
	err := s.store.SetChatBackendArrangement(storage.ChatBackends{
		Mode: string(after.Mode), Order: after.Order, Voice: after.Voice, Off: after.Off,
		UpdatedAt: time.Now(),
	})
	if err != nil {
		s.restore(before)
		return err
	}
	s.log.Info().Strs("order", after.Order).Strs("voice", after.Voice).Strs("off", after.Off).
		Str("mode", string(after.Mode)).Msg("chat_backends_arranged")
	return nil
}

// ResetBackends forgets the owner's arrangement and returns to the
// environment's.
func (s *Service) ResetBackends() error {
	if s.pool == nil {
		return ErrNoPool
	}
	if err := s.store.ClearChatBackendArrangement(); err != nil {
		return err
	}
	s.restore(s.poolDefaults)
	s.log.Info().Msg("chat_backends_reset")
	return nil
}

// restore puts the pool back to an arrangement exactly: every backend not
// named off is switched on.
func (s *Service) restore(st ai.Settings) {
	s.pool.SetMode(st.Mode)
	_ = s.pool.SetOrder(st.Order)
	_ = s.pool.SetVoiceOrder(st.Voice)
	for _, name := range st.Order {
		if !slices.Contains(st.Off, name) {
			_ = s.pool.SetOff(name, false)
		}
	}
	for _, name := range st.Off {
		_ = s.pool.SetOff(name, true)
	}
}
