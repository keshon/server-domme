package mind

import "sync"

// maxEncounters bounds the table. Reaching it clears the whole thing rather
// than evicting one entry: the data is a short-lived courtesy record, and
// losing it costs one forced reply per person, which is the safe direction.
const maxEncounters = 4096

// encounter is what happened last time this person approached.
type encounter struct {
	ignoredLast bool
}

// Encounters remembers who has approached the character and how it went.
//
// It exists to serve the two rails in Decide that stop deliberate silence
// reading as a broken bot: never ignore a first approach, never ignore the
// same person twice running. Both need memory that Decide itself cannot hold,
// because Decide is a pure function.
//
// Deliberately in memory. Losing it on restart means the next person to speak
// gets answered for certain, which is a fine way to come back online.
type Encounters struct {
	mu   sync.Mutex
	seen map[string]encounter
}

// NewEncounters returns an empty table.
func NewEncounters() *Encounters {
	return &Encounters{seen: make(map[string]encounter)}
}

// Approach reports what Decide needs to know about this person's history.
// The key is the caller's to build; it should identify a person within a
// channel, so being ignored in one place does not force a reply in another.
func (e *Encounters) Approach(key string) (firstApproach, ignoredLast bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	prior, known := e.seen[key]
	return !known, prior.ignoredLast
}

// Record stores how an approach was resolved.
func (e *Encounters) Record(key string, outcome Outcome) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.seen) >= maxEncounters {
		e.seen = make(map[string]encounter)
	}
	e.seen[key] = encounter{ignoredLast: outcome == OutcomeIgnore}
}

// Len reports how many people are remembered.
func (e *Encounters) Len() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.seen)
}
