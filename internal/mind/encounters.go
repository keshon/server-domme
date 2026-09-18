package mind

import "sync"

// maxEncounters bounds the table. Reaching it clears the whole thing rather
// than evicting one entry: the data is a short-lived courtesy record, and
// losing it costs one forced reply per person, which is the safe direction.
const maxEncounters = 4096

// encounter is what happened last time this person approached.
type encounter struct {
	ignoredLast bool
	// ignoredDirect is whether that ignored approach addressed her directly.
	// Only then is approaching again pushing; see IgnoredDirectly.
	ignoredDirect bool
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

// IgnoredDirectly reports whether the approach she last ignored from this
// person addressed her directly — a mention, a reply to her, her name.
//
// Pressing again after that is pushing. Pressing again after she let an
// untagged line go is not: tagging her is how anyone repairs a message that
// was not noticed, and counting it as pestering had her ignore someone and
// then grow annoyed that they noticed.
func (e *Encounters) IgnoredDirectly(key string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.seen[key].ignoredDirect
}

// Record stores how an approach was resolved.
func (e *Encounters) Record(key string, outcome Outcome, trigger Trigger) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.seen) >= maxEncounters {
		e.seen = make(map[string]encounter)
	}
	ignored := outcome == OutcomeIgnore
	direct := trigger == TriggerMention || trigger == TriggerReply || trigger == TriggerNamed
	e.seen[key] = encounter{ignoredLast: ignored, ignoredDirect: ignored && direct}
}

// Len reports how many people are remembered.
func (e *Encounters) Len() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.seen)
}
