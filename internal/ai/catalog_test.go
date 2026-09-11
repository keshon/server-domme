package ai

import "testing"

// Several models share one donated machine and go down with it, so a pool
// built from three entries on one server is one failure from empty — the exact
// situation Pool exists to prevent.
func TestPickModelsTakesOneEntryPerServer(t *testing.T) {
	entries := []CatalogEntry{
		{ID: "srv_a:big", Server: "srv_a", Requests: 900},
		{ID: "srv_a:small", Server: "srv_a", Requests: 800},
		{ID: "srv_b:mid", Server: "srv_b", Requests: 700},
		{ID: "srv_c:tiny", Server: "srv_c", Requests: 100},
	}

	picked := PickModels(entries, 3)
	if len(picked) != 3 {
		t.Fatalf("picked %d entries, want 3", len(picked))
	}

	seen := map[string]bool{}
	for _, e := range picked {
		if seen[e.Server] {
			t.Fatalf("server %q picked twice: %+v", e.Server, picked)
		}
		seen[e.Server] = true
	}
}

func TestPickModelsRanksByCallCount(t *testing.T) {
	entries := []CatalogEntry{
		{ID: "srv_a:quiet", Server: "srv_a", Requests: 5},
		{ID: "srv_b:busy", Server: "srv_b", Requests: 900},
	}
	picked := PickModels(entries, 1)
	if len(picked) != 1 || picked[0].ID != "srv_b:busy" {
		t.Fatalf("picked %+v, want the entry with the higher call count", picked)
	}
}

// "auto" routes to a random public server that may not serve the model it is
// handed, and answers model_not_allowed when it does not.
func TestPickModelsSkipsAutoAndUnqualifiedEntries(t *testing.T) {
	entries := []CatalogEntry{
		{ID: "auto", Requests: 9999},
		{ID: "", Server: "srv_x", Requests: 500},
		{ID: "srv_b:real", Server: "srv_b", Requests: 10},
	}
	picked := PickModels(entries, 3)
	if len(picked) != 1 || picked[0].ID != "srv_b:real" {
		t.Fatalf("picked %+v, want only the server-qualified entry", picked)
	}
}

func TestPickModelsHandlesFewerEntriesThanAsked(t *testing.T) {
	picked := PickModels([]CatalogEntry{{ID: "srv_a:one", Server: "srv_a"}}, 5)
	if len(picked) != 1 {
		t.Fatalf("picked %d entries, want 1", len(picked))
	}
	if got := PickModels(nil, 3); len(got) != 0 {
		t.Fatalf("picked %d entries from an empty catalogue, want 0", len(got))
	}
}
