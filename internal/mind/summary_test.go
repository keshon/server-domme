package mind

import (
	"strings"
	"testing"
	"time"
)

func turn(user, name, content string, at time.Time) Turn {
	return Turn{UserID: user, Username: name, Content: content, At: at}
}

// Two labelled lines rather than JSON, because the experiment this came from
// failed to parse a third of its structured replies against a model it
// controlled.
func TestParseSummaryReadsTheLabelledLines(t *testing.T) {
	gist, detail, ok := ParseSummary(
		"GIST: argument about the purge rules\nDETAIL: cass and newbie went at it for an hour.")
	if !ok {
		t.Fatal("failed to parse a well-formed reply")
	}
	if gist != "argument about the purge rules" {
		t.Errorf("gist = %q", gist)
	}
	if !strings.Contains(detail, "went at it") {
		t.Errorf("detail = %q", detail)
	}
}

// Models wrap things. None of this should cost a memory.
func TestParseSummaryToleratesDecoration(t *testing.T) {
	for name, reply := range map[string]string{
		"lowercase":     "gist: the rules row\ndetail: it ran late.",
		"markdown bold": "**GIST:** the rules row\n**DETAIL:** it ran late.",
		"quoted":        `GIST: "the rules row"` + "\nDETAIL: 'it ran late.'",
		"with preamble": "Here is your note:\n\nGIST: the rules row\nDETAIL: it ran late.",
		"bulleted":      "- GIST: the rules row\n- DETAIL: it ran late.",
	} {
		t.Run(name, func(t *testing.T) {
			gist, _, ok := ParseSummary(reply)
			if !ok {
				t.Fatalf("failed to parse: %q", reply)
			}
			if gist != "the rules row" {
				t.Errorf("gist = %q, want the decoration stripped", gist)
			}
		})
	}
}

// A memory that cannot be read is simply not remembered.
func TestParseSummaryFailsQuietlyWithoutAGist(t *testing.T) {
	for _, reply := range []string{
		"",
		"I'm sorry, I can't help with that.",
		"DETAIL: plenty of detail and nothing to call it",
		"{\"gist\": \"json instead\"}",
	} {
		if _, _, ok := ParseSummary(reply); ok {
			t.Errorf("claimed to parse %q", reply)
		}
	}
}

func TestParseSummaryBoundsWhatItStores(t *testing.T) {
	long := strings.Repeat("x", maxDetailChars*3)
	gist, detail, ok := ParseSummary("GIST: " + long + "\nDETAIL: " + long)
	if !ok {
		t.Fatal("failed to parse")
	}
	if len([]rune(gist)) > maxGistChars {
		t.Errorf("gist is %d runes, want at most %d", len([]rune(gist)), maxGistChars)
	}
	if len([]rune(detail)) > maxDetailChars {
		t.Errorf("detail is %d runes, want at most %d", len([]rune(detail)), maxDetailChars)
	}
}

func TestWeighMomentRatesABusyExchangeAboveAPassingOne(t *testing.T) {
	now := time.Now()

	passing := []Turn{turn("u1", "cass", "morning", now)}

	var busy []Turn
	for i := 0; i < 20; i++ {
		busy = append(busy, Turn{
			UserID: string(rune('a' + i%5)), Username: "someone",
			Content: "a real exchange", At: now, Mentioned: i%3 == 0,
		})
	}

	if WeighMoment(busy) <= WeighMoment(passing) {
		t.Errorf("a busy exchange should weigh more: %.2f vs %.2f",
			WeighMoment(busy), WeighMoment(passing))
	}
	if got := WeighMoment(nil); got != 0 {
		t.Errorf("nothing happened: weight = %.2f, want 0", got)
	}
}

// Summarising while people are still talking produces a memory of half an
// argument, then another of the other half.
func TestSettledWaitsForThePause(t *testing.T) {
	now := time.Now()
	quiet := 5 * time.Minute

	talking := []Turn{turn("u1", "cass", "and another thing", now.Add(-time.Minute))}
	finished := []Turn{turn("u1", "cass", "anyway", now.Add(-10*time.Minute))}

	if Settled(talking, now, quiet) {
		t.Error("summarised a conversation still in progress")
	}
	if !Settled(finished, now, quiet) {
		t.Error("refused to summarise a finished conversation")
	}
	if Settled(nil, now, quiet) {
		t.Error("claimed an empty channel had settled into something")
	}
}

func TestParticipantsExcludesHerOwnTurns(t *testing.T) {
	now := time.Now()
	got := Participants([]Turn{
		turn("u1", "cass", "hi", now),
		{Content: "hello", At: now, FromBot: true},
		turn("u1", "cass", "again", now),
		turn("u2", "newbie", "hi", now),
	})

	if len(got) != 2 {
		t.Fatalf("got %v, want two distinct people and not herself", got)
	}
}

func TestSummaryPromptCarriesTheTranscriptAndNotTheCharacter(t *testing.T) {
	now := time.Now()
	msgs := SummaryPrompt([]Turn{
		turn("u1", "cass", "what about the pins", now),
		{Content: "leave them", At: now, FromBot: true},
	})

	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want a system instruction and a transcript", len(msgs))
	}
	if !strings.Contains(msgs[1].Content, "cass: what about the pins") {
		t.Errorf("transcript missing the speaker: %q", msgs[1].Content)
	}
	if !strings.Contains(msgs[1].Content, "you: leave them") {
		t.Errorf("her own turns should be attributed to her: %q", msgs[1].Content)
	}
}
