package shortlink

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// shutdownGrace bounds how long in-flight redirects may finish once shutdown
// starts. A redirect is a single write with no upstream call behind it, so this
// only has to cover the write itself — a longer grace would just delay exit.
const shutdownGrace = 5 * time.Second

// RunServer serves short-link redirects until ctx is cancelled, then drains and
// returns. It blocks, so run it in a goroutine.
//
// A failure here is logged and returned rather than fatal: the redirect server
// is a side channel, and losing it should not take the Discord bot down with
// it.
func RunServer(ctx context.Context, store *storage.Storage, cfg *config.Config, log zerolog.Logger) error {
	mux := http.NewServeMux()
	if cfg != nil && cfg.HealthCheckPath != "" {
		registerHealthCheck(mux, cfg.HealthCheckPath)
	}
	mux.HandleFunc("/", redirectHandler(store, log))

	addr := cfg.ShortLinkAddr
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		log.Info().Msg("shortlink_server_stopping")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Warn().Err(err).Msg("shortlink_server_shutdown_failed")
		}
	}()

	log.Info().Str("addr", addr).Msg("shortlink_server_listening")
	err := srv.ListenAndServe()
	<-shutdownDone

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error().Err(err).Msg("shortlink_server_exited")
		return err
	}
	log.Info().Msg("shortlink_server_stopped")
	return nil
}

func redirectHandler(store *storage.Storage, log zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/")
		if id == "" {
			http.NotFound(w, r)
			return
		}

		guildID, link, err := store.FindLinkByID(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if err := store.IncrementClicks(guildID, id); err != nil {
			log.Warn().Str("short_id", id).Err(err).Msg("shortlink_click_count_failed")
		}

		log.Info().
			Str("short_id", id).
			Str("guild_id", guildID).
			Str("target", link.Original).
			Msg("shortlink_redirected")
		http.Redirect(w, r, link.Original, http.StatusSeeOther)
	}
}

func registerHealthCheck(mux *http.ServeMux, path string) {
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("pong"))
		}
	})
}
