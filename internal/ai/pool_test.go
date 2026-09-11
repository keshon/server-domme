package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func testLogger() zerolog.Logger { return zerolog.Nop() }

// okServer answers every call with reply, counting hits.
func okServer(t *testing.T, reply string, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"`+reply+`"}}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// deadServer refuses every call, counting hits.
func deadServer(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPoolFailsOverToTheNextBackend(t *testing.T) {
	var deadHits, liveHits atomic.Int64
	dead := deadServer(t, &deadHits)
	live := okServer(t, "second answered", &liveHits)

	pool := NewPool(testLogger(),
		NewClient("dead", dead.URL, "m", ""),
		NewClient("live", live.URL, "m", ""),
	)

	got, err := pool.Generate(context.Background(), nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "second answered" {
		t.Errorf("Generate = %q, want the healthy backend's reply", got)
	}
	if deadHits.Load() != attemptsPerTry {
		t.Errorf("dead backend tried %d times, want %d", deadHits.Load(), attemptsPerTry)
	}
}

// A backend that just failed must not be retried on the next message, or a
// dead relay costs two wasted round trips per reply for as long as it is down.
func TestPoolSkipsACooledDownBackend(t *testing.T) {
	var deadHits, liveHits atomic.Int64
	dead := deadServer(t, &deadHits)
	live := okServer(t, "ok", &liveHits)

	pool := NewPool(testLogger(),
		NewClient("dead", dead.URL, "m", ""),
		NewClient("live", live.URL, "m", ""),
	)

	for i := 0; i < 3; i++ {
		if _, err := pool.Generate(context.Background(), nil); err != nil {
			t.Fatalf("Generate %d: %v", i, err)
		}
	}

	if deadHits.Load() != attemptsPerTry {
		t.Errorf("dead backend tried %d times across three calls, want %d — cooldown did not hold",
			deadHits.Load(), attemptsPerTry)
	}
	if liveHits.Load() != 3 {
		t.Errorf("live backend served %d calls, want 3", liveHits.Load())
	}
}

// Refusing to speak because every relay misbehaved once is worse than trying
// the least-bad one, so an all-cooled pool still attempts a call.
func TestPoolStillTriesWhenEveryBackendIsCooledDown(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fail the first round, recover afterwards.
		if hits.Add(1) <= attemptsPerTry {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"back"}}]}`)
	}))
	defer srv.Close()

	pool := NewPool(testLogger(), NewClient("only", srv.URL, "m", ""))

	if _, err := pool.Generate(context.Background(), nil); err == nil {
		t.Fatal("first call should have failed")
	}
	got, err := pool.Generate(context.Background(), nil)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got != "back" {
		t.Errorf("Generate = %q, want the recovered reply", got)
	}
}

func TestPoolReportsNoBackendWhenAllFail(t *testing.T) {
	var hits atomic.Int64
	dead := deadServer(t, &hits)

	pool := NewPool(testLogger(), NewClient("dead", dead.URL, "m", ""))
	_, err := pool.Generate(context.Background(), nil)
	if !errors.Is(err, ErrNoBackend) {
		t.Fatalf("Generate err = %v, want it to wrap ErrNoBackend", err)
	}
}

// A cancelled context is the caller giving up. Walking the rest of the pool
// would run past the deadline the caller set.
func TestPoolStopsOnContextCancellation(t *testing.T) {
	var firstHits, secondHits atomic.Int64

	// The handler is released by the test rather than by the request context:
	// on Windows the server side is not cancelled when the client abandons the
	// read, so waiting on r.Context() here blocks Close forever.
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstHits.Add(1)
		<-release
	}))
	defer slow.Close()
	defer close(release)
	second := okServer(t, "never", &secondHits)

	pool := NewPool(testLogger(),
		NewClient("slow", slow.URL, "m", ""),
		NewClient("second", second.URL, "m", ""),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if _, err := pool.Generate(ctx, nil); err == nil {
		t.Fatal("Generate should have failed on the cancelled context")
	}
	if secondHits.Load() != 0 {
		t.Errorf("second backend was tried %d times after cancellation, want 0", secondHits.Load())
	}
}

func TestPoolPrefersTheBackendThatKeepsAnswering(t *testing.T) {
	var flakyHits, goodHits atomic.Int64
	flaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if flakyHits.Add(1)%2 == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"flaky"}}]}`)
	}))
	defer flaky.Close()
	good := okServer(t, "good", &goodHits)

	pool := NewPool(testLogger(),
		NewClient("flaky", flaky.URL, "m", ""),
		NewClient("good", good.URL, "m", ""),
	)

	for i := 0; i < 6; i++ {
		if _, err := pool.Generate(context.Background(), nil); err != nil {
			t.Fatalf("Generate %d: %v", i, err)
		}
	}

	stats := pool.Stats()
	if stats[0].Name != "good" {
		t.Errorf("top-ranked backend is %q, want %q", stats[0].Name, "good")
	}
}

func TestPoolGenerateIsSafeUnderConcurrentUse(t *testing.T) {
	var hits atomic.Int64
	srv := okServer(t, "ok", &hits)
	pool := NewPool(testLogger(), NewClient("a", srv.URL, "m", ""))

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = pool.Generate(context.Background(), nil)
			_ = pool.Stats()
		}()
	}
	wg.Wait()
}

func TestBuildRejectsAnEmptyPool(t *testing.T) {
	_, err := Build(context.Background(), testLogger(), Options{})
	if !errors.Is(err, ErrNoBackend) {
		t.Fatalf("Build err = %v, want ErrNoBackend when nothing is enabled", err)
	}
}
