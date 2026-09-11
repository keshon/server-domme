// Package ai talks to OpenAI-compatible chat completion backends.
//
// The bot speaks through free public relays (pollinations, g4f.space), which
// are donated infrastructure with no availability guarantee: a backend that
// answered a minute ago may refuse the model, return a different one than it
// was asked for, or vanish. Pool is the answer to that — it holds several
// Clients and fails over between them, scoring each on what it has actually
// done rather than on what it advertises.
//
// Nothing here knows about Discord or about the character speaking. Building
// the messages is internal/mind's job; this package only delivers them.
package ai

import (
	"context"
	"errors"
)

// Message is one chat turn in the OpenAI wire format.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Message.Role values. These go out on the wire, so they are the backend's
// spelling rather than ours.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Provider generates one reply for a conversation. Implemented by Client
// against a single backend and by Pool across several.
type Provider interface {
	Generate(ctx context.Context, messages []Message) (string, error)
}

// ErrNoBackend is returned when every configured backend failed or is in
// cooldown. Callers distinguish it from a transport error because it means
// "stay quiet for now", not "retry this one".
var ErrNoBackend = errors.New("no backend available")

// ErrBackendRefused marks a failure the backend will keep producing until
// something changes outside this process: no credit, no key, not allowed.
//
// It is separated from an ordinary failure because retrying is not merely
// useless but actively harmful — each attempt costs a request, and in a
// channel it costs a typing indicator that promises a reply the bot cannot
// produce. A refused backend is therefore rested for far longer than a flaky
// one. Seen in production as 402 "No cake credits" from g4f.space, whose
// anonymous allowance is earned per IP in a browser and which a server
// therefore never has.
var ErrBackendRefused = errors.New("backend refused the request")

// ErrEmptyReply is returned when a backend answered successfully with nothing
// usable. It counts as a failure against that backend's score: a relay that
// returns 200 with an empty body is broken in a way a status code does not
// report.
var ErrEmptyReply = errors.New("backend returned an empty reply")
