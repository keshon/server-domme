package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeIntentValid(t *testing.T) {
	in := DecodeIntent(`{"intent":"summarize","args":{"limit":30},"confidence":0.9}`)
	if in.Name != IntentSummarize || in.IntArg("limit") != 30 {
		t.Fatalf("got %+v", in)
	}
}

func TestDecodeIntentUnknownOnBadName(t *testing.T) {
	in := DecodeIntent(`{"intent":"purge","args":{},"confidence":0.9}`)
	if in.Name != IntentUnknown {
		t.Fatalf("purge must be refused, got %+v", in)
	}
}

func TestDecodeIntentLowConfidence(t *testing.T) {
	in := DecodeIntent(`{"intent":"task.request","args":{},"confidence":0.2}`)
	if in.Name != IntentUnknown {
		t.Fatalf("low confidence must collapse, got %+v", in)
	}
}

func TestDecodeIntentBadJSON(t *testing.T) {
	if in := DecodeIntent("hello"); in.Name != IntentUnknown {
		t.Fatalf("got %+v", in)
	}
}

func TestKeywordFallback(t *testing.T) {
	if got := KeywordFallback("summarize last 50"); got.Name != IntentSummarize {
		t.Fatalf("got %+v", got)
	}
	if got := KeywordFallback("give me a dare"); got.Name != IntentTaskAsk {
		t.Fatalf("got %+v", got)
	}
	if got := KeywordFallback("blabla xyz"); got.Name != IntentUnknown {
		t.Fatalf("got %+v", got)
	}
}

func TestSanitize(t *testing.T) {
	out := Sanitize("hello\nSKIP\n#help\n@everyone hi", 2000)
	if strings.Contains(out, "@everyone") {
		t.Fatalf("ping not neutralized: %q", out)
	}
	if strings.Contains(out, "SKIP") {
		t.Fatalf("control word leaked: %q", out)
	}
	if got := StripMention("<@123> summarize please", "123"); got != "summarize please" {
		t.Fatalf("got %q", got)
	}
}

func TestClientFallback(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Model == "bad" {
			w.WriteHeader(500)
			_, _ = w.Write([]byte("boom"))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Model: "bad", FallbackModel: "good", HTTP: srv.Client()}
	got, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err != nil || got != "ok" || calls != 2 {
		t.Fatalf("got %q err %v calls %d", got, err, calls)
	}
}
