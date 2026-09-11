package ai

import (
	"strings"
	"testing"
)

func TestParseBackendSpecAcceptsThreeAndFourFields(t *testing.T) {
	withoutKey, err := ParseBackendSpec("ollama|http://localhost:11434/v1|llama3.1")
	if err != nil {
		t.Fatalf("ParseBackendSpec: %v", err)
	}
	if withoutKey.Name != "ollama" || withoutKey.Model != "llama3.1" {
		t.Errorf("parsed %+v", withoutKey)
	}
	if withoutKey.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", withoutKey.APIKey)
	}

	withKey, err := ParseBackendSpec("paid|https://api.example.com/v1|gpt-4o-mini|sk-abc123")
	if err != nil {
		t.Fatalf("ParseBackendSpec: %v", err)
	}
	if withKey.APIKey != "sk-abc123" {
		t.Errorf("APIKey = %q", withKey.APIKey)
	}
}

// Operators write these by hand in an env file, so surrounding whitespace is
// the norm rather than the exception.
func TestParseBackendSpecTrimsWhitespace(t *testing.T) {
	got, err := ParseBackendSpec(" ollama | http://localhost:11434/v1 | llama3.1 ")
	if err != nil {
		t.Fatalf("ParseBackendSpec: %v", err)
	}
	if got.Name != "ollama" || got.BaseURL != "http://localhost:11434/v1" || got.Model != "llama3.1" {
		t.Errorf("parsed %+v", got)
	}
}

// A trailing slash would otherwise produce "…/v1//chat/completions".
func TestParseBackendSpecNormalisesTheBaseURL(t *testing.T) {
	got, err := ParseBackendSpec("x|http://host/v1/|m")
	if err != nil {
		t.Fatalf("ParseBackendSpec: %v", err)
	}
	if strings.HasSuffix(got.BaseURL, "/") {
		t.Errorf("BaseURL = %q, want the trailing slash gone", got.BaseURL)
	}
}

func TestParseBackendSpecRejectsMalformedEntries(t *testing.T) {
	cases := map[string]string{
		"too few fields":  "ollama|http://localhost:11434/v1",
		"too many fields": "a|http://h/v1|m|k|extra",
		"empty name":      "|http://h/v1|m",
		"empty base url":  "a||m",
		"empty model":     "a|http://h/v1|",
		"no scheme":       "a|localhost:11434/v1|m",
		"nothing at all":  "",
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseBackendSpec(spec); err == nil {
				t.Errorf("ParseBackendSpec(%q) accepted a malformed spec", spec)
			}
		})
	}
}
