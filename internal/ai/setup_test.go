package ai

import (
	"context"
	"testing"
	"time"
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
