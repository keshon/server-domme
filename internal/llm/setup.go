package llm

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/keshon/server-domme/internal/config"
	"github.com/rs/zerolog"
)

// Options configures which backends the pool holds.
type Options struct {
	// CustomBaseURL points at an explicit OpenAI-compatible endpoint — a
	// local Ollama or LM Studio, or a paid API. Tried first when set,
	// because naming one is a deliberate choice.
	CustomBaseURL  string
	CustomModel    string
	CustomFallback string
	CustomAPIKey   string

	// UsePollinations adds text.pollinations.ai. Anonymously it answers a
	// trivial prompt fine but refuses a real one with 402
	// KEY_BUDGET_EXHAUSTED — an assembled prompt runs about four kilobytes,
	// which already costs more than the anonymous allowance. With a key it
	// works; point CustomBaseURL at it instead, with CustomAPIKey set.
	UsePollinations bool
	// UseG4F adds models discovered from the g4f.space relay catalogue.
	UseG4F bool
	// G4FPicks caps how many g4f.space models to add. Zero means
	// DefaultG4FPicks.
	G4FPicks  int
	G4FAPIKey string

	// Extra holds additional backends as "name|baseURL|model|key" specs, most
	// preferred first. See ParseBackendSpec.
	Extra []string
	// Mode is how the pool orders backends; empty is ModePriority.
	Mode Mode

	Timeout   time.Duration
	MaxTokens int
}

// Ready reports whether an LLM call may be attempted: enabled plus at least
// one backend — an explicit endpoint, a free relay, or an extra spec.
func Ready(cfg *config.Config) bool {
	if cfg == nil || !cfg.LLMEnabled {
		return false
	}
	if cfg.LLMBaseURL != "" && cfg.LLMModel != "" {
		return true
	}
	if cfg.LLMUseG4F || cfg.LLMUsePollinations {
		return true
	}
	for _, s := range cfg.LLMBackends {
		if strings.TrimSpace(s) != "" {
			return true
		}
	}
	return false
}

// backendSetupHint is what callers show when Ready is false.
const backendSetupHint = "Set `LLM_ENABLED=true` with `LLM_BASE_URL` and `LLM_MODEL`, or enable a free relay with `LLM_USE_G4F=true`."

// SetupHint returns the operator-facing hint for the unconfigured case.
func SetupHint() string { return backendSetupHint }

// OptionsFromConfig reads pool options off Config.
func OptionsFromConfig(cfg *config.Config) Options {
	var o Options
	if cfg == nil {
		return o
	}
	timeout := 20 * time.Second
	if cfg.LLMTimeoutSec > 0 {
		timeout = time.Duration(cfg.LLMTimeoutSec) * time.Second
	}
	maxTokens := 512
	if cfg.LLMMaxTokens > 0 {
		maxTokens = cfg.LLMMaxTokens
	}
	return Options{
		CustomBaseURL:   strings.TrimRight(cfg.LLMBaseURL, "/"),
		CustomModel:     cfg.LLMModel,
		CustomFallback:  cfg.LLMFallbackModel,
		CustomAPIKey:    cfg.LLMAPIKey,
		UsePollinations: cfg.LLMUsePollinations,
		UseG4F:          cfg.LLMUseG4F,
		G4FPicks:        cfg.LLMG4FPicks,
		G4FAPIKey:       cfg.LLMG4FAPIKey,
		Extra:           cfg.LLMBackends,
		Mode:            mustMode(cfg.LLMBackendOrder),
		Timeout:         timeout,
		MaxTokens:       maxTokens,
	}
}

func mustMode(s string) Mode {
	m, _ := ParseMode(s)
	return m
}

// Build assembles a Pool from opts, without contacting the network: g4f.space
// catalogue picks join later via FillG4F, so construction stays synchronous
// and per-request construction stays cheap.
//
// A backend that cannot be set up is skipped rather than failing the build.
// Only an empty pool is an error.
func Build(log zerolog.Logger, opts Options) (*Pool, error) {
	var clients []*Client

	if opts.CustomBaseURL != "" && opts.CustomModel != "" {
		c := NewClient("custom", opts.CustomBaseURL, opts.CustomModel, opts.CustomAPIKey)
		c.FallbackModel = opts.CustomFallback
		clients = append(clients, c)
	}

	for _, spec := range opts.Extra {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		client, err := ParseBackendSpec(spec)
		if err != nil {
			log.Error().Err(err).Msg("llm_backend_spec_invalid")
			continue
		}
		clients = append(clients, client)
		log.Debug().Str("backend", client.Name).Str("model", client.Model).Msg("llm_backend_added")
	}

	if opts.UsePollinations {
		clients = append(clients, NewClient("pollinations", PollinationsBaseURL, PollinationsModel, ""))
	}

	for _, c := range clients {
		if opts.Timeout > 0 {
			c.Timeout = opts.Timeout
			c.HTTP.Timeout = opts.Timeout + 5*time.Second
		}
		if opts.MaxTokens > 0 {
			c.MaxTokens = opts.MaxTokens
		}
	}

	pool := NewPool(log, clients...)
	pool.SetMode(opts.Mode)
	if pool.Len() == 0 && !opts.UseG4F {
		return nil, ErrNoBackend
	}
	if pool.Len() > 0 {
		log.Info().Int("backends", pool.Len()).Str("mode", string(pool.Mode())).Msg("llm_pool_ready")
	}
	return pool, nil
}

// FillG4F discovers g4f.space models and adds them to an existing pool. It
// runs off the request path (at startup or in the background): a slow catalog
// costs nothing once the pool already answers through other backends.
func FillG4F(ctx context.Context, log zerolog.Logger, pool *Pool, opts Options) {
	if pool == nil || !opts.UseG4F {
		return
	}
	picks := opts.G4FPicks
	if picks <= 0 {
		picks = DefaultG4FPicks
	}
	entries, err := FetchCatalog(ctx, G4FBaseURL, opts.G4FAPIKey)
	if err != nil {
		log.Warn().Err(err).Msg("llm_catalog_fetch_failed")
		return
	}
	added := 0
	for _, e := range PickModels(entries, picks) {
		c := NewClient("g4f:"+e.OwnedBy, G4FBaseURL, e.ID, opts.G4FAPIKey)
		if opts.Timeout > 0 {
			c.Timeout = opts.Timeout
			c.HTTP.Timeout = opts.Timeout + 5*time.Second
		}
		if opts.MaxTokens > 0 {
			c.MaxTokens = opts.MaxTokens
		}
		pool.Add(c)
		added++
		log.Debug().
			Str("model", e.ID).
			Str("owned_by", e.OwnedBy).
			Int("relay_requests", e.Requests).
			Msg("llm_backend_added")
	}
	log.Info().Int("backends", pool.Len()).Int("g4f_added", added).Msg("llm_pool_g4f_ready")
}

var (
	sharedMu      sync.Mutex
	sharedPool    *Pool
	sharedKey     string
	sharedG4FDone map[string]bool
)

func fingerprint(o Options) string {
	var b strings.Builder
	b.WriteString(o.CustomBaseURL + "\x00" + o.CustomModel + "\x00" + o.CustomFallback + "\x00")
	b.WriteString(o.CustomAPIKey + "\x00")
	if o.UsePollinations {
		b.WriteString("p1\x00")
	}
	if o.UseG4F {
		b.WriteString("g1\x00")
	}
	b.WriteString(strings.Join(o.Extra, ",") + "\x00")
	b.WriteString(string(o.Mode) + "\x00")
	return b.String()
}

// ProviderForConfig returns the shared pool for cfg, building it once per
// distinct configuration. g4f.space catalogue discovery runs in the
// background so the first call never waits on it: early requests use the
// explicit endpoint and extra backends, later ones include the relays too.
//
// The pool carries scores and cooldowns across every caller (slash commands
// and the mention router share it), so a relay that fails for one is already
// cooling down for the next.
func ProviderForConfig(cfg *config.Config) Provider {
	opts := OptionsFromConfig(cfg)
	key := fingerprint(opts)

	sharedMu.Lock()
	if sharedPool != nil && sharedKey == key {
		p := sharedPool
		sharedMu.Unlock()
		return p
	}
	if sharedG4FDone == nil {
		sharedG4FDone = make(map[string]bool)
	}
	log := zerolog.Nop()
	pool, err := Build(log, opts)
	if err != nil || pool.Len() == 0 {
		// No synchronous backends: still create an (empty) pool so G4F can
		// fill it, unless G4F is off too — then fall back to a single custom
		// client that reports "not configured" per call.
		if !opts.UseG4F {
			sharedMu.Unlock()
			return legacyClient(cfg)
		}
		pool = NewPool(log)
	}
	sharedPool = pool
	sharedKey = key
	already := sharedG4FDone[key]
	if opts.UseG4F && !already {
		sharedG4FDone[key] = true
		go FillG4F(context.Background(), log, pool, opts)
	}
	p := sharedPool
	sharedMu.Unlock()
	return p
}

// legacyClient preserves the old single-endpoint behavior for configurations
// with no poolable backends at all.
func legacyClient(cfg *config.Config) Provider {
	return NewFromConfig(cfg)
}

// resetSharedForTests drops the shared pool. Tests only.
func resetSharedForTests() {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	sharedPool = nil
	sharedKey = ""
	sharedG4FDone = make(map[string]bool)
}
