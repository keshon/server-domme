package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The shapes below were all observed coming back from the free relays during
// development, not invented: the relays disagree with each other and with the
// OpenAI spec, and parseReply exists to absorb that.
func TestParseReplyHandlesEveryObservedShape(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "openai chat completion",
			body: `{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`,
			want: "hello",
		},
		{
			name: "line-delimited stream despite stream:false",
			body: `{"message":{"content":"he"},"done":false}` + "\n" +
				`{"message":{"content":"llo"},"done":true}`,
			want: "hello",
		},
		{
			name: "delta rather than message",
			body: `{"choices":[{"delta":{"content":"partial"}}]}`,
			want: "partial",
		},
		{
			name: "plain text, not json at all",
			body: `just talking`,
			want: "just talking",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseReply("test", []byte(tc.body))
			if err != nil {
				t.Fatalf("parseReply: %v", err)
			}
			if got != tc.want {
				t.Errorf("parseReply = %q, want %q", got, tc.want)
			}
		})
	}
}

// A relay can refuse inside a 200 body. Treating that as a successful empty
// reply would post nothing to the channel and score the backend as healthy.
func TestParseReplyFailsOnErrorInsideOK(t *testing.T) {
	body := `{"error":{"message":"Model 'x' is not allowed on this server","type":"model_not_allowed"}}`
	if _, err := parseReply("test", []byte(body)); err == nil {
		t.Fatal("parseReply accepted an application-level error as a reply")
	}
}

func TestClientGenerateSendsAndParses(t *testing.T) {
	var gotBody chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("posted to %q, want /chat/completions", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"answered"}}]}`)
	}))
	defer srv.Close()

	c := NewClient("test", srv.URL, "some-model", "")
	got, err := c.Generate(context.Background(), []Message{{Role: RoleUser, Content: "hi"}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "answered" {
		t.Errorf("Generate = %q, want %q", got, "answered")
	}
	if gotBody.Model != "some-model" {
		t.Errorf("sent model %q, want %q", gotBody.Model, "some-model")
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Content != "hi" {
		t.Errorf("sent messages %+v, want the one passed in", gotBody.Messages)
	}
}

func TestClientGenerateSendsAPIKeyOnlyWhenSet(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	withKey := NewClient("test", srv.URL, "m", "secret")
	if _, err := withKey.Generate(context.Background(), nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if auth != "Bearer secret" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer secret")
	}

	noKey := NewClient("test", srv.URL, "m", "")
	if _, err := noKey.Generate(context.Background(), nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if auth != "" {
		t.Errorf("Authorization = %q, want it absent when no key is set", auth)
	}
}

func TestClientGenerateRejectsEmptyReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":""}}]}`)
	}))
	defer srv.Close()

	c := NewClient("test", srv.URL, "m", "")
	_, err := c.Generate(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), ErrEmptyReply.Error()) {
		t.Fatalf("Generate err = %v, want it to report an empty reply", err)
	}
}

func TestClientGenerateReportsHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `slow down`)
	}))
	defer srv.Close()

	c := NewClient("test", srv.URL, "m", "")
	_, err := c.Generate(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("Generate err = %v, want the status code reported", err)
	}
}

// Left out when unset, so a relay samples the way she was tuned against;
// sent as given when set.
func TestClientSendsTemperatureOnlyWhenSet(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	c := NewClient("test", srv.URL, "m", "")
	if _, err := c.Generate(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "temperature") {
		t.Errorf("sent a temperature nobody set: %s", raw)
	}

	warm := 0.9
	c.Temperature = &warm
	if _, err := c.Generate(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	var got chatRequest
	_ = json.Unmarshal(raw, &got)
	if got.Temperature == nil || *got.Temperature != 0.9 {
		t.Errorf("temperature sent as %v, want 0.9", got.Temperature)
	}
}

// The persona's private appraisal comes back as JSON, and Clean cuts every
// line that opens like "name: " — which is every line of an object. Asked
// raw, it arrives whole; asked the ordinary way, it is cleaned as speech. The
// per-call temperature reaches the wire either way.
func TestRawAndTemperatureArePerCall(t *testing.T) {
	object := "{\n\"read\": \"sincere\",\n\"act\": \"reply\"\n}"
	var sent []float64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Temperature *float64 `json:"temperature"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Temperature != nil {
			sent = append(sent, *body.Temperature)
		}
		reply, _ := json.Marshal(object)
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%s}}]}`, reply)
	}))
	defer srv.Close()
	c := NewClient("test", srv.URL, "m", "")

	raw, err := c.Generate(WithRaw(WithTemperature(context.Background(), 0.4)), nil)
	if err != nil || raw != object {
		t.Fatalf("raw came back as %q, %v", raw, err)
	}
	cleaned, err := c.Generate(context.Background(), nil)
	if err != nil || cleaned == object {
		t.Fatalf("an ordinary call was not cleaned: %q, %v", cleaned, err)
	}
	if len(sent) != 1 || sent[0] != 0.4 {
		t.Errorf("temperatures sent: %v", sent)
	}
}
