package confess

import (
	"testing"
)

func TestDecodeVerdictRisky(t *testing.T) {
	v := DecodeVerdict(`{"risky":true,"reason":"credible threat"}`)
	if !v.Risky || v.Reason == "" {
		t.Fatalf("got %+v", v)
	}
}

func TestDecodeVerdictSafe(t *testing.T) {
	v := DecodeVerdict(`{"risky":false,"reason":"whatever"}`)
	if v.Risky || v.Reason != "" {
		t.Fatalf("got %+v", v)
	}
}

func TestDecodeVerdictBadJSON(t *testing.T) {
	if v := DecodeVerdict("nope"); v.Risky {
		t.Fatalf("got %+v", v)
	}
}
