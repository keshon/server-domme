package ai

import (
	"context"

	"github.com/rs/zerolog"
)

// Default backend coordinates.
//
// Both are free public services. They are donated infrastructure that can
// change or disappear without notice, which is why the endpoints are constants
// here rather than spread across the call sites that would all need editing at
// once — the previous version of this code pointed at g4f.dev, which has since
// become a static site that answers 405 to every request.
const (
	PollinationsBaseURL = "https://text.pollinations.ai/openai"
	PollinationsModel   = "openai"
	G4FBaseURL          = "https://g4f.space/v1"
)

// DefaultG4FPicks is how many g4f.space models go into the pool. Each one is
// on a different donated server (see PickModels), so this is really a count of
// independent machines to fall back through.
const DefaultG4FPicks = 3

// Options configures which backends Build puts in the pool.
type Options struct {
	// UsePollinations adds text.pollinations.ai. Off by default, and the
	// reason is worth knowing before turning it on: anonymously it answers a
	// trivial prompt fine but refuses a real one with 402
	// KEY_BUDGET_EXHAUSTED — an assembled character prompt runs about four
	// kilobytes, which already costs more than the anonymous allowance.
	// Measured, not inferred. With a key it works; point CustomBaseURL at it
	// instead, with CustomAPIKey set.
	UsePollinations bool
	// UseG4F adds models discovered from the g4f.space relay catalogue.
	UseG4F bool
	// G4FPicks caps how many g4f.space models to add. Zero means
	// DefaultG4FPicks.
	G4FPicks int

	// CustomBaseURL points at any other OpenAI-compatible endpoint — a local
	// Ollama or LM Studio, or a paid API. Set it when the free relays are not
	// good enough for what the character has to sound like; it is tried
	// first, because naming one is an explicit choice and the defaults are
	// not.
	CustomBaseURL string
	CustomModel   string
	CustomAPIKey  string
}

// Build assembles a Pool from opts, discovering g4f.space models if asked.
//
// A backend that cannot be set up is logged and skipped rather than failing
// the build: losing the relay catalogue is a reason to speak through
// pollinations, not a reason for the bot not to start. Only an empty pool is
// an error, and the caller decides what that means.
func Build(ctx context.Context, log zerolog.Logger, opts Options) (*Pool, error) {
	var clients []*Client

	if opts.CustomBaseURL != "" {
		clients = append(clients, NewClient("custom",
			opts.CustomBaseURL, opts.CustomModel, opts.CustomAPIKey))
	}

	if opts.UseG4F {
		picks := opts.G4FPicks
		if picks <= 0 {
			picks = DefaultG4FPicks
		}
		entries, err := FetchCatalog(ctx, G4FBaseURL)
		if err != nil {
			log.Warn().Err(err).Msg("ai_catalog_fetch_failed")
		} else {
			for _, e := range PickModels(entries, picks) {
				clients = append(clients, NewClient("g4f:"+e.OwnedBy, G4FBaseURL, e.ID, ""))
				log.Debug().
					Str("model", e.ID).
					Str("owned_by", e.OwnedBy).
					Int("relay_requests", e.Requests).
					Msg("ai_backend_added")
			}
		}
	}

	if opts.UsePollinations {
		clients = append(clients, NewClient("pollinations",
			PollinationsBaseURL, PollinationsModel, ""))
	}

	pool := NewPool(log, clients...)
	if pool.Len() == 0 {
		return nil, ErrNoBackend
	}

	log.Info().Int("backends", pool.Len()).Msg("ai_pool_ready")
	return pool, nil
}
