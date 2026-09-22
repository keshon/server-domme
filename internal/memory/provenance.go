package memory

import (
	"strings"
)

// Kind is how she came to know something: its epistemic type. See
// docs/persona-v3.md, workstream I.
//
// The kinds rank, strongest first: what the author wrote about her, what
// actually happened, what someone (she included) said, and what she made of
// it. A lower kind never overwrites a higher one, and nothing derived is ever
// stronger than what it rests on.
type Kind string

// Kinds. Written into her memory files, so frozen once shipped.
const (
	Authored    Kind = "authored"
	Observed    Kind = "observed"
	Stated      Kind = "stated"
	Interpreted Kind = "interpreted"
)

// rank is a kind's strength; higher is stronger, and an unknown kind is the
// weakest there is.
func (k Kind) rank() int {
	switch k {
	case Authored:
		return 4
	case Observed:
		return 3
	case Stated:
		return 2
	case Interpreted:
		return 1
	}
	return 0
}

// Outranks reports whether k is strictly stronger than o.
func (k Kind) Outranks(o Kind) bool { return k.rank() > o.rank() }

// Valid reports whether k is one of the four kinds.
func (k Kind) Valid() bool { return k.rank() > 0 }

// Derived is the kind of something derived from things of the given kinds:
// never stronger than the weakest of them, and never stronger than
// interpreted, since deriving is interpreting. It is the invariant that
// derived information never gains strength.
func Derived(from ...Kind) Kind {
	weakest := Interpreted
	for _, k := range from {
		if k.rank() < weakest.rank() {
			weakest = k
		}
	}
	return weakest
}

// Source is where one remembered thing came from: its kind, and a reference
// to what it rests on — "msg 1234" for a message, "day 2026-09-22" for a day
// reflected on. The reference is for the code and for a person reading the
// file; it is never shown to the model, since a message reference carries a
// Discord id.
type Source struct {
	Kind Kind
	Ref  string
}

// Message is a source resting on one Discord message.
func Message(kind Kind, messageID string) Source {
	if messageID == "" {
		return Source{Kind: kind}
	}
	return Source{Kind: kind, Ref: "msg " + messageID}
}

// OnDay is a source resting on a day she reflected on.
func OnDay(kind Kind, day string) Source {
	return Source{Kind: kind, Ref: "day " + day}
}

// String writes a source as it appears in a file: "stated msg 1234".
func (s Source) String() string {
	if !s.Kind.Valid() {
		return ""
	}
	if s.Ref == "" {
		return string(s.Kind)
	}
	return string(s.Kind) + " " + oneLine(s.Ref)
}

// IsZero reports whether no source is recorded.
func (s Source) IsZero() bool { return s.Kind == "" }

// ParseSource reads what String wrote. The bool is false for anything that
// does not start with a kind, which is how a hand-written line without a
// source is told apart from one with.
func ParseSource(text string) (Source, bool) {
	text = strings.TrimSpace(text)
	kind, ref, _ := strings.Cut(text, " ")
	k := Kind(strings.ToLower(kind))
	if !k.Valid() {
		return Source{}, false
	}
	return Source{Kind: k, Ref: strings.TrimSpace(ref)}, true
}

// sourceSep divides an item's text from its source on a line: "text · stated
// msg 1234". A middle dot, because it reads as punctuation to a person and
// rarely appears in what she writes.
const sourceSep = " · "

// withSource appends a source to an item's text.
func withSource(text string, src Source) string {
	if s := src.String(); s != "" {
		return oneLine(text) + sourceSep + s
	}
	return oneLine(text)
}

// splitSource takes a trailing source off an item's text, if it has one.
func splitSource(item string) (string, Source) {
	i := strings.LastIndex(item, sourceSep)
	if i < 0 {
		return item, Source{}
	}
	if src, ok := ParseSource(item[i+len(sourceSep):]); ok {
		return strings.TrimSpace(item[:i]), src
	}
	return item, Source{}
}
