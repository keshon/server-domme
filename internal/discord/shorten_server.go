package discord

import (
	"log"
	"net/http"

	"server-domme/internal/storage"
)

// shortenServer starts a lightweight HTTP server that resolves short links to their original URLs.
func shortenServer(storage *storage.Storage) {
	log.Printf("[INFO] Starting shortlink redirect server...")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[1:] // remove leading "/"
		if id == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		records := storage.GetRecordsList()
		for _, record := range records {
			for _, link := range record.ShortLinks {
				if link.ShortID == id {
					http.Redirect(w, r, link.Original, http.StatusSeeOther)
					log.Printf("Redirected short link %s → %s", id, link.Original)
					return
				}
			}
		}

		http.NotFound(w, r)
	})

	addr := ":8787"
	log.Printf("[INFO] Shortlink redirect server listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
