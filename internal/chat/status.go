package chat

import "github.com/keshon/server-domme/internal/ai"

// statReporter is the part of a provider that can describe itself. Pool
// implements it; a bare Client does not, and a test double need not.
type statReporter interface {
	Stats() []ai.BackendStat
}

// Status is an operator-facing snapshot of the persona.
type Status struct {
	// Backends is how each configured backend has been behaving, best first.
	// Empty when the provider does not report stats.
	Backends []ai.BackendStat
	// Waiting is how many approaches are held for a later attempt.
	Waiting int
	// Queued is how many approaches are waiting for a worker.
	Queued int
}

// Status reports what the service is doing. It is read-only and safe to call
// from a command handler.
func (s *Service) Status() Status {
	st := Status{
		Waiting: s.deferrals.Len(),
		Queued:  len(s.work),
	}
	if reporter, ok := s.provider.(statReporter); ok {
		st.Backends = reporter.Stats()
	}
	return st
}

// CharacterName reports who the persona is, for commands that describe her.
func (s *Service) CharacterName() string {
	if s.character == nil {
		return ""
	}
	return s.character.Name
}
