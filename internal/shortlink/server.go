package shortlink

import (
	"log"
	"net/http"

	"server-domme/internal/storage"
)

// RunServer starts a lightweight HTTP server that resolves short links to their original URLs.
// It blocks until the server exits; typically run in a goroutine.
func RunServer(store *storage.Storage) {
	log.Printf("[INFO] Starting shortlink redirect server...")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
	log.Printf("[INFO] Shortlink redirect server listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
