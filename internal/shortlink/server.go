package shortlink

import (
	"context"
	"log"
	"net/http"

	"server-domme/internal/storage"
)

// RunServer starts a lightweight HTTP server that resolves short links to their original URLs.
// It blocks until the server exits or ctx is cancelled; run in a goroutine.
func RunServer(store *storage.Storage) {
	RunServerWithContext(context.Background(), store)
}

// RunServerWithContext starts the shortlink HTTP server and respects ctx for graceful shutdown.
func RunServerWithContext(ctx context.Context, store *storage.Storage) {
	log.Printf("[INFO] Starting shortlink redirect server...")

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[1:] // remove leading "/"
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
			log.Printf("[WARN] Failed to increment clicks for %s: %v", id, err)
		}

		log.Printf("[INFO] Redirected short link %s → %s (guild=%s)", id, link.Original, guildID)
		http.Redirect(w, r, link.Original, http.StatusSeeOther)
	})

	addr := ":8787"
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		<-ctx.Done()
		log.Println("[INFO] Shutting down shortlink server...")
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()

	log.Printf("[INFO] Shortlink redirect server listening on %s\n", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		// Log the error but do NOT call log.Fatal — that would kill the whole process.
		log.Printf("[ERR] Shortlink server exited: %v", err)
	}
}
