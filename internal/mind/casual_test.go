package mind

import "testing"

func TestCasualTypesLikeAPerson(t *testing.T) {
	plain := CasualStyle{}
	cases := map[string]string{
		"yeah.":                            "yeah",
		"fine. i am out of chickens.":      "fine. i am out of chickens",
		"sure...":                          "sure...",
		"why not?":                         "why not?",
		"not tonight — ask me tomorrow":    "not tonight - ask me tomorrow",
		"it’s “fine”…":                     `it's "fine"...`,
		"i heard you; i just do not care.": "i heard you, i just do not care",
		"here: https://example.com/a;b.":   "here: https://example.com/a;b.",
		"run `go test ./...` and see.":     "run `go test ./...` and see.",
	}
	for in, want := range cases {
		if got := Casual(in, plain, 0.99); got != want {
			t.Errorf("Casual(%q) = %q, want %q", in, got, want)
		}
	}
}

// A full stop is a signal when people mostly leave it off. Short with
// someone, she keeps it, and her contractions.
func TestCasualKeepsTheFullStopWhenSheIsCurt(t *testing.T) {
	curt := CasualStyle{Curt: true, SlipChance: 1}
	if got := Casual("don't.", curt, 0); got != "don't." {
		t.Errorf("curt reply became %q", got)
	}
}

func TestCasualSlipsOnlyTheSafeContractions(t *testing.T) {
	slips := CasualStyle{SlipChance: 1}
	got := Casual("i'm sure you're fine, but we're not and i'll say so. that's all", slips, 0)
	want := "im sure youre fine, but we're not and i'll say so. thats all"
	if got != want {
		t.Errorf("slipped to %q, want %q", got, want)
	}
	if got := Casual("don't", CasualStyle{SlipChance: 0.2}, 0.5); got != "don't" {
		t.Errorf("slipped on a roll above the chance: %q", got)
	}
}
