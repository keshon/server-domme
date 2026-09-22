package chat

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	"github.com/keshon/server-domme/internal/memory"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// relay answers every call with its own name.
func relay(t *testing.T, name string) *ai.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"`+name+`"}}]}`)
	}))
	t.Cleanup(srv.Close)
	return ai.NewClient(name, srv.URL, "m", "")
}

func poolService(t *testing.T, store *storage.Storage, now func() time.Time, names ...string) (*Service, *ai.Pool) {
	t.Helper()
	var clients []*ai.Client
	for _, n := range names {
		clients = append(clients, relay(t, n))
	}
	pool := ai.NewPool(zerolog.Nop(), clients...)
	mem, err := memory.Open(t.TempDir(), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	svc := New(Deps{
		Character: &mind.Character{Name: "Domme"},
		Provider:  pool, Voice: pool.Voice(), Pool: pool,
		Storage: store, Memory: mem, Log: zerolog.Nop(), Now: now,
	})
	return svc, pool
}

func testStore(t *testing.T) *storage.Storage {
	t.Helper()
	store, err := storage.NewStorage(t.TempDir(), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// What the developer arranges with /chat backends survives a restart: a new
// service over a fresh pool comes up arranged the same way.
func TestArrangedBackendsSurviveARestart(t *testing.T) {
	store := testStore(t)
	svc, _ := poolService(t, store, time.Now, "a", "b", "c")
	err := svc.ArrangeBackends(func(p *ai.Pool) error {
		if err := p.SetOrder([]string{"c"}); err != nil {
			return err
		}
		if err := p.SetVoiceOrder([]string{"b"}); err != nil {
			return err
		}
		return p.SetOff("a", true)
	})
	if err != nil {
		t.Fatal(err)
	}

	_, pool := poolService(t, store, time.Now, "a", "b", "c")
	st := pool.Settings()
	if st.Order[0] != "c" || len(st.Voice) != 1 || st.Voice[0] != "b" || len(st.Off) != 1 || st.Off[0] != "a" {
		t.Errorf("after restart: %+v", st)
	}
}

// A change that fails part way leaves the pool as it was, and nothing is
// stored.
func TestAFailedArrangementChangesNothing(t *testing.T) {
	store := testStore(t)
	svc, pool := poolService(t, store, time.Now, "a", "b")
	err := svc.ArrangeBackends(func(p *ai.Pool) error {
		_ = p.SetOrder([]string{"b"})
		return p.SetVoiceOrder([]string{"nope"})
	})
	if !errors.Is(err, ai.ErrUnknownBackend) {
		t.Fatalf("err = %v, want ErrUnknownBackend", err)
	}
	if got := pool.Settings().Order[0]; got != "a" {
		t.Errorf("order starts with %q after a failed change, want a", got)
	}
	if _, stored := store.ChatBackendArrangement(); stored {
		t.Error("a failed change was stored")
	}
}

func TestResetReturnsToTheDeploymentsArrangement(t *testing.T) {
	store := testStore(t)
	svc, pool := poolService(t, store, time.Now, "a", "b")
	_ = svc.ArrangeBackends(func(p *ai.Pool) error {
		_ = p.SetOrder([]string{"b"})
		return p.SetOff("a", true)
	})
	if err := svc.ResetBackends(); err != nil {
		t.Fatal(err)
	}
	st := pool.Settings()
	if st.Order[0] != "a" || len(st.Off) != 0 {
		t.Errorf("after reset: %+v", st)
	}
	if _, stored := store.ChatBackendArrangement(); stored {
		t.Error("reset left an arrangement stored")
	}
}

// Her voice stays on the backend it last spoke through for the session,
// even once the preferred one is healthy again.
func TestVoiceKeepsItsBackendForTheSession(t *testing.T) {
	store := testStore(t)
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	svc, _ := poolService(t, store, func() time.Time { return now }, "first", "second")
	scene := mind.Scene{Trigger: mind.TriggerMention, Username: "Big M", Now: now}
	a := mind.Appraisal{Act: mind.ActReply, Intent: "say hi"}

	// As if a failover had just put her voice on second.
	svc.voiceName, svc.voiceAt = "second", now

	_, backend, err := svc.speak(context.Background(), scene, mind.Known{}, a, "")
	if err != nil {
		t.Fatal(err)
	}
	if backend != "second" {
		t.Errorf("within the session, spoke through %q, want second", backend)
	}

	now = now.Add(voiceSession + time.Second)
	_, backend, err = svc.speak(context.Background(), scene, mind.Known{}, a, "")
	if err != nil {
		t.Fatal(err)
	}
	if backend != "first" {
		t.Errorf("after the session, spoke through %q, want first again", backend)
	}
}
