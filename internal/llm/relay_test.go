package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/keshon/server-domme/internal/config"
	"github.com/rs/zerolog"
)

func TestParseBackendSpec(t *testing.T) {
	c, err := ParseBackendSpec("home|http://localhost:11434/v1|llama3.1")
	if err != nil || c.Name != "home" || c.Model != "llama3.1" {
		t.Fatalf("got %+v err %v", c, err)
	}
	c, err = ParseBackendSpec("paid|https://api.example.com/v1|gpt-4o-mini|sk-x")
	if err != nil || c.APIKey != "sk-x" {
		t.Fatalf("key not parsed: %+v err %v", c, err)
	}
	for _, bad := range []string{"a|b", "a|b|c|d|e", "|http://x|m", "n|ftp://x|m", "n||m"} {
		if _, err := ParseBackendSpec(bad); err == nil {
			t.Fatalf("spec %q must fail", bad)
		}
	}
}

func TestPickModelsDiverseServers(t *testing.T) {
	entries := []CatalogEntry{
		{ID: "auto", Server: "s0", Requests: 9999},
		{ID: "s1:m1", Server: "s1", Requests: 100, OwnedBy: "a"},
		{ID: "s1:m2", Server: "s1", Requests: 90, OwnedBy: "a"},
		{ID: "s2:m1", Server: "s2", Requests: 50, OwnedBy: "b"},
		{ID: "", Server: "s3", Requests: 10},
	}
	picked := PickModels(entries, 3)
	if len(picked) != 2 {
		t.Fatalf("got %+v", picked)
	}
	if picked[0].ID != "s1:m1" || picked[1].ID != "s2:m1" {
		t.Fatalf("ranking or diversity wrong: %+v", picked)
	}
}

func TestParseReplyShapes(t *testing.T) {
	// Single OpenAI completion.
	got, err := parseReply("t", []byte(`{"choices":[{"message":{"content":" hi "}}]}`))
	if err != nil || strings.TrimSpace(got) != "hi" {
		t.Fatalf("openai: %q %v", got, err)
	}
	// Streamed chunks despite stream:false.
	stream := `{"choices":[{"delta":{"content":"hel"}}]}` + "\n" + `{"choices":[{"delta":{"content":"lo"}}]}`
	got, err = parseReply("t", []byte(stream))
	if err != nil || got != "hello" {
		t.Fatalf("stream: %q %v", got, err)
	}
	// Ollama-native shape.
	got, err = parseReply("t", []byte(`{"message":{"content":"yo"}}`))
	if err != nil || got != "yo" {
		t.Fatalf("ollama: %q %v", got, err)
	}
	// Plain text passthrough.
	got, err = parseReply("t", []byte(`just words`))
	if err != nil || got != "just words" {
		t.Fatalf("plain: %q %v", got, err)
	}
	// App-level error inside 200.
	if _, err = parseReply("t", []byte(`{"error":{"message":"nope","type":"x"}}`)); err == nil {
		t.Fatal("app error must fail")
	}
}

func TestPoolFailoverToSecond(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("boom"))
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer good.Close()

	b1 := NewClient("bad", bad.URL, "m", "")
	b1.HTTP = bad.Client()
	b2 := NewClient("good", good.URL, "m", "")
	b2.HTTP = good.Client()
	p := NewPool(zerolog.Nop(), b1, b2)

	got, name, err := p.CompleteNamed(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil || got != "ok" || name != "good" {
		t.Fatalf("got %q %q err %v", got, name, err)
	}
	stats := p.Stats()
	if len(stats) != 2 || stats[0].Failures == 0 || stats[1].Successes == 0 {
		t.Fatalf("stats not learned: %+v", stats)
	}
}

func TestPoolRefusedSkipsRetryAndCools(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(402)
		_, _ = w.Write([]byte("no cake"))
	}))
	defer srv.Close()
	b := NewClient("ref", srv.URL, "m", "")
	b.HTTP = srv.Client()
	p := NewPool(zerolog.Nop(), b)
	_, _, err := p.CompleteNamed(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if !errors.Is(err, ErrNoBackend) {
		t.Fatalf("want ErrNoBackend, got %v", err)
	}
	if !errors.Is(err, ErrBackendRefused) {
		t.Fatalf("want wrapped ErrBackendRefused, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("refused backend must get 1 attempt, got %d", calls)
	}
	if st := p.Stats()[0]; st.CooledFor <= 0 {
		t.Fatalf("refused backend must cool, got %+v", st)
	}
}

func TestPoolAllDownGivesNoBackend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()
	b := NewClient("x", srv.URL, "m", "")
	b.HTTP = srv.Client()
	p := NewPool(zerolog.Nop(), b)
	if _, err := p.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil); !errors.Is(err, ErrNoBackend) {
		t.Fatalf("got %v", err)
	}
}

func TestReadyWithFreeRelayOnly(t *testing.T) {
	cfg := &config.Config{LLMEnabled: true, LLMUseG4F: true}
	if !Ready(cfg) {
		t.Fatal("UseG4F alone must be ready")
	}
	cfg = &config.Config{LLMEnabled: true}
	if Ready(cfg) {
		t.Fatal("no backend must not be ready")
	}
	cfg = &config.Config{LLMEnabled: false, LLMUseG4F: true}
	if Ready(cfg) {
		t.Fatal("disabled must not be ready")
	}
}

func TestBuildWithExtraSpec(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"` + req.Model + `"}}]}`))
	}))
	defer srv.Close()

	pool, err := Build(zerolog.Nop(), Options{
		Extra: []string{"home|" + srv.URL + "|llama3.1", "bad spec"},
		Mode:  ModePriority,
	})
	if err != nil || pool.Len() != 1 {
		t.Fatalf("pool %+v err %v", pool, err)
	}
	got, err := pool.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil || got != "llama3.1" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestProviderForConfigShared(t *testing.T) {
	resetSharedForTests()
	cfg := &config.Config{LLMEnabled: true, LLMBaseURL: "http://localhost:9/v1", LLMModel: "m"}
	a := ProviderForConfig(cfg)
	b := ProviderForConfig(cfg)
	if a != b {
		t.Fatal("same config must share one pool")
	}
	resetSharedForTests()
}
