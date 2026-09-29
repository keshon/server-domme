package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Free relay coordinates.
//
// Both are free public services. They are donated infrastructure that can
// change or disappear without notice, which is why the endpoints are constants
// here rather than spread across the call sites that would all need editing at
// once.
const (
	PollinationsBaseURL = "https://text.pollinations.ai/openai"
	PollinationsModel   = "openai"
	G4FBaseURL          = "https://g4f.space/v1"
)

// DefaultG4FPicks is how many g4f.space models go into the pool. Each one is
// on a different donated server (see PickModels), so this is really a count of
// independent machines to fall back through.
const DefaultG4FPicks = 3

// catalogTimeout bounds the model list fetch. It runs at startup and on a slow
// refresh, never in the path of a reply, so a slow answer costs nothing.
const catalogTimeout = 20 * time.Second

// CatalogEntry is one model a relay offers.
type CatalogEntry struct {
	// ID is what goes in a request's "model" field. On g4f.space it is
	// "<server>:<model>".
	ID string `json:"id"`
	// Model is the bare model name, which the relay does not guarantee to
	// honour: treat it as a hint for choosing, never as a promise.
	Model string `json:"model"`
	// OwnedBy names whoever donated the server. Free-text, operator-supplied.
	OwnedBy string `json:"owned_by"`
	// Server identifies the donated machine behind this entry. Several models
	// share one, and they fail together when it goes down.
	Server string `json:"server"`
	// Requests is the relay's lifetime call count for this entry. It is the
	// only liveness signal the catalogue carries.
	Requests int `json:"requests"`
}

type catalogResponse struct {
	Data []CatalogEntry `json:"data"`
}

// FetchCatalog reads the model list from an OpenAI-compatible relay.
func FetchCatalog(ctx context.Context, baseURL, apiKey string) ([]CatalogEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()

	url := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("llm: build catalog request: %w", err)
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: fetch catalog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: catalog returned %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: read catalog: %w", err)
	}

	var parsed catalogResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("llm: decode catalog: %w", err)
	}
	return parsed.Data, nil
}

// PickModels chooses up to n model ids to put behind a Pool.
//
// Entries are ranked by call count, because a donated server nobody has called
// successfully is indistinguishable from a dead one and the catalogue offers
// no other health signal. Then at most one entry per server is taken: several
// models share one machine and go down with it, so three picks from one server
// is one failure away from an empty pool. The "auto" entry is skipped: it
// routes to a random public server that may not serve the model it is handed.
func PickModels(entries []CatalogEntry, n int) []CatalogEntry {
	ranked := make([]CatalogEntry, 0, len(entries))
	for _, e := range entries {
		if e.Server == "" || e.ID == "" || e.ID == "auto" {
			continue
		}
		ranked = append(ranked, e)
	}

	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && ranked[j].Requests > ranked[j-1].Requests; j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}

	seen := make(map[string]bool, n)
	picked := make([]CatalogEntry, 0, n)
	for _, e := range ranked {
		if len(picked) >= n {
			break
		}
		if seen[e.Server] {
			continue
		}
		seen[e.Server] = true
		picked = append(picked, e)
	}
	return picked
}
