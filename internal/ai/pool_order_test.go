package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// answering is a backend whose reply is its own name, so a test can tell who
// spoke.
func answering(t *testing.T, name string, hits *atomic.Int64) *Client {
	t.Helper()
	return NewClient(name, okServer(t, name, hits).URL, "m", "")
}

func speaker(t *testing.T, p interface {
	GenerateNamed(context.Context, []Message) (string, string, error)
}, ctx context.Context) string {
	t.Helper()
	_, name, err := p.GenerateNamed(ctx, nil)
	if err != nil {
		t.Fatalf("GenerateNamed: %v", err)
	}
	return name
}

// The point of priority mode: the backend the operator put first keeps
// answering, even when another has a better record.
func TestPriorityModeKeepsTheConfiguredOrder(t *testing.T) {
	var a, b atomic.Int64
	pool := NewPool(testLogger(), answering(t, "first", &a), answering(t, "second", &b))

	for i := 0; i < 5; i++ {
		if got := speaker(t, pool, context.Background()); got != "first" {
			t.Fatalf("call %d answered by %q, want first", i, got)
		}
	}
	if b.Load() != 0 {
		t.Errorf("second backend served %d calls while first was healthy", b.Load())
	}
}

// A failover lasts only as long as the cooldown: the preferred backend takes
// its place back once it is rested.
func TestPriorityModeReturnsToThePreferredBackendAfterCooldown(t *testing.T) {
	var fails atomic.Bool
	fails.Store(true)
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fails.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"back"}}]}`))
	}))
	defer flaky.Close()
	var hits atomic.Int64
	pool := NewPool(testLogger(), NewClient("first", flaky.URL, "m", ""), answering(t, "second", &hits))

	if got := speaker(t, pool, context.Background()); got != "second" {
		t.Fatalf("with first failing, answered by %q, want second", got)
	}
	fails.Store(false)
	pool.mu.Lock()
	pool.backends[0].cooldownUntil = time.Time{}
	pool.mu.Unlock()
	if got := speaker(t, pool, context.Background()); got != "first" {
		t.Errorf("after the cooldown, answered by %q, want first back", got)
	}
}

func TestCooldownDoublesWithEachFailureInARow(t *testing.T) {
	want := []time.Duration{90 * time.Second, 3 * time.Minute, 6 * time.Minute, 12 * time.Minute, 24 * time.Minute, 30 * time.Minute, 30 * time.Minute}
	for i, w := range want {
		if got := cooldownFor(i + 1); got != w {
			t.Errorf("cooldown after %d in a row = %s, want %s", i+1, got, w)
		}
	}
}

func TestASuccessClearsTheCooldownStreak(t *testing.T) {
	var hits atomic.Int64
	pool := NewPool(testLogger(), answering(t, "a", &hits))
	b := pool.backends[0]
	now := time.Now()
	pool.recordFailure(b, errors.New("x"), attemptsPerTry, now)
	pool.recordFailure(b, errors.New("x"), attemptsPerTry, now)
	if b.streak != 2 {
		t.Fatalf("streak = %d after two cooldowns, want 2", b.streak)
	}
	pool.recordSuccess(b)
	if b.streak != 0 || !b.cooldownUntil.IsZero() {
		t.Errorf("after a success: streak %d, cooldown %v; want both cleared", b.streak, b.cooldownUntil)
	}
}

// The voice has its own order; thinking keeps the general one.
func TestVoiceUsesItsOwnOrder(t *testing.T) {
	var a, b atomic.Int64
	pool := NewPool(testLogger(), answering(t, "thinker", &a), answering(t, "speaker", &b))
	if err := pool.SetVoiceOrder([]string{"speaker"}); err != nil {
		t.Fatal(err)
	}

	if got := speaker(t, pool, context.Background()); got != "thinker" {
		t.Errorf("thinking answered by %q, want thinker", got)
	}
	if got := speaker(t, pool.Voice(), context.Background()); got != "speaker" {
		t.Errorf("voice answered by %q, want speaker", got)
	}
}

// A voice order is a preference, not a cage: with every voice backend down,
// she still speaks through another rather than going silent.
func TestVoiceFallsBackPastItsOrder(t *testing.T) {
	var deadHits, liveHits atomic.Int64
	dead := deadServer(t, &deadHits)
	pool := NewPool(testLogger(), NewClient("voice", dead.URL, "m", ""), answering(t, "other", &liveHits))
	if err := pool.SetVoiceOrder([]string{"voice"}); err != nil {
		t.Fatal(err)
	}
	if got := speaker(t, pool.Voice(), context.Background()); got != "other" {
		t.Errorf("with the voice backend dead, answered by %q, want other", got)
	}
}

// Stickiness: a conversation that failed over stays on the backend it failed
// over to, so her voice does not change mid-conversation.
func TestPreferKeepsAFailedOverVoice(t *testing.T) {
	var a, b atomic.Int64
	pool := NewPool(testLogger(), answering(t, "first", &a), answering(t, "second", &b))
	ctx := WithPrefer(context.Background(), "second")
	if got := speaker(t, pool.Voice(), ctx); got != "second" {
		t.Errorf("preferring second, answered by %q", got)
	}
}

func TestPreferIsIgnoredWhileThatBackendRests(t *testing.T) {
	var a, b atomic.Int64
	pool := NewPool(testLogger(), answering(t, "first", &a), answering(t, "second", &b))
	pool.mu.Lock()
	pool.backends[1].cooldownUntil = time.Now().Add(time.Hour)
	pool.mu.Unlock()
	ctx := WithPrefer(context.Background(), "second")
	if got := speaker(t, pool, ctx); got != "first" {
		t.Errorf("preferred backend resting, answered by %q, want first", got)
	}
}

func TestASwitchedOffBackendIsNeverTried(t *testing.T) {
	var a, b atomic.Int64
	pool := NewPool(testLogger(), answering(t, "first", &a), answering(t, "second", &b))
	if err := pool.SetOff("first", true); err != nil {
		t.Fatal(err)
	}
	if got := speaker(t, pool, context.Background()); got != "second" {
		t.Errorf("answered by %q, want second", got)
	}
	if a.Load() != 0 {
		t.Errorf("switched-off backend was called %d times", a.Load())
	}
}

func TestTheLastBackendCannotBeSwitchedOff(t *testing.T) {
	var a, b atomic.Int64
	pool := NewPool(testLogger(), answering(t, "first", &a), answering(t, "second", &b))
	if err := pool.SetOff("first", true); err != nil {
		t.Fatal(err)
	}
	if err := pool.SetOff("second", true); !errors.Is(err, ErrLastBackend) {
		t.Errorf("switching off the last one: err = %v, want ErrLastBackend", err)
	}
}

func TestSetOrderPutsTheNamedFirstAndKeepsTheRest(t *testing.T) {
	pool := NewPool(testLogger(),
		NewClient("a", "http://a", "m", ""), NewClient("b", "http://b", "m", ""),
		NewClient("c", "http://c", "m", ""), NewClient("d", "http://d", "m", ""))
	if err := pool.SetOrder([]string{"c", "a"}); err != nil {
		t.Fatal(err)
	}
	got := pool.Settings().Order
	want := []string{"c", "a", "b", "d"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if err := pool.SetOrder([]string{"nope"}); !errors.Is(err, ErrUnknownBackend) {
		t.Errorf("unknown name: err = %v, want ErrUnknownBackend", err)
	}
}

// A stored arrangement can outlive the backends it names; restoring it must
// not fail, only skip and say what it skipped.
func TestApplySkipsNamesThePoolNoLongerHas(t *testing.T) {
	pool := NewPool(testLogger(), NewClient("a", "http://a", "m", ""), NewClient("b", "http://b", "m", ""))
	unknown := pool.Apply(Settings{Order: []string{"gone", "b"}, Voice: []string{"b", "gone"}, Off: []string{"a"}})
	if len(unknown) != 2 {
		t.Errorf("unknown = %v, want gone reported twice", unknown)
	}
	st := pool.Settings()
	if st.Order[0] != "b" || len(st.Voice) != 1 || st.Voice[0] != "b" || len(st.Off) != 1 || st.Off[0] != "a" {
		t.Errorf("settings after apply = %+v", st)
	}
}

func TestDuplicateNamesAreMadeUnique(t *testing.T) {
	pool := NewPool(testLogger(), NewClient("g4f", "http://a", "m", ""), NewClient("g4f", "http://b", "m", ""))
	st := pool.Settings()
	if st.Order[0] == st.Order[1] {
		t.Errorf("two backends share the name %q", st.Order[0])
	}
}
