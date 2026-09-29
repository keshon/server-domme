package llm

import (
	"context"
	"errors"
)

// Provider generates one reply for a conversation. Implemented by Client
// against a single backend and by Pool across several free relays plus the
// explicit endpoint.
type Provider interface {
	Complete(ctx context.Context, messages []Message, temperature *float64) (string, error)
}

// ErrNoBackend is returned when every configured backend failed or is in
// cooldown. Callers fail closed on it: deterministic behavior plus a setup
// hint, never an empty reply presented as success.
var ErrNoBackend = errors.New("no backend available")

// ErrBackendRefused marks a failure the backend will keep producing until
// something changes outside this process: no credit, no key, not allowed.
//
// Separated from an ordinary failure because retrying within one request is
// not merely useless but actively harmful on free relays — each attempt costs
// a request against a donated allowance. A refused backend moves straight to
// the next one and rests far longer than a flaky one.
var ErrBackendRefused = errors.New("backend refused the request")

// ErrEmptyReply is returned when a backend answered successfully with nothing
// usable. It counts as a failure against that backend's score: a relay that
// returns 200 with an empty body is broken in a way a status code does not
// report.
var ErrEmptyReply = errors.New("backend returned an empty reply")
