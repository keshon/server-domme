package ai

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// A backend on the other end of a home connection is a different animal from
// a relay, so the deadline has to be reachable from configuration.
func TestBuildAppliesTheConfiguredTimeout(t *testing.T) {
	pool, err := Build(context.Background(), testLogger(), Options{
		CustomBaseURL: "http://localhost:11434/v1",
		CustomModel:   "llama3.1",
		Timeout:       5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, c := range pool.backends {
		if c.client.HTTP.Timeout != 5*time.Minute {
			t.Errorf("%s timeout = %v, want the configured 5m", c.client.Name, c.client.HTTP.Timeout)
		}
	}
}

func TestBuildKeepsTheDefaultTimeoutWhenUnset(t *testing.T) {
	pool, err := Build(context.Background(), testLogger(), Options{
		CustomBaseURL: "http://localhost:11434/v1",
		CustomModel:   "llama3.1",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, c := range pool.backends {
		if c.client.HTTP.Timeout != DefaultTimeout {
			t.Errorf("%s timeout = %v, want DefaultTimeout", c.client.Name, c.client.HTTP.Timeout)
		}
	}
}

// The only question this log line is ever asked is "did my backend make it
// into the pool", and a count cannot answer it: a spec dropped as malformed
// looks exactly like one that was never configured.
func TestBuildNamesTheBackendsItAssembled(t *testing.T) {
	var logged bytes.Buffer
	log := zerolog.New(&logged)

	_, err := Build(context.Background(), log, Options{
		CustomBaseURL: "http://localhost:11434/v1",
		CustomModel:   "llama3.1",
		Extra:         []string{"mine|http://g4f:8080/v1|gpt-4o-mini"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	out := logged.String()
	for _, want := range []string{"ai_pool_ready", "custom", "mine"} {
		if !strings.Contains(out, want) {
			t.Errorf("startup log does not mention %q:\n%s", want, out)
		}
	}
}
