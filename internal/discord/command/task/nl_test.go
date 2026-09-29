package task

import (
	"context"
	"testing"
)

func TestDecodeRequestSpec(t *testing.T) {
	s := DecodeRequestSpec(`{"duration_min":10,"keywords":["quick","easy"],"intensity":"gentle"}`)
	if s.DurationMin != 10 || len(s.Keywords) != 2 || s.Intensity != "gentle" {
		t.Fatalf("got %+v", s)
	}
}

func TestDecodeRequestSpecBadJSON(t *testing.T) {
	if s := DecodeRequestSpec("nope"); len(s.Keywords) != 0 || s.DurationMin != 0 {
		t.Fatalf("got %+v", s)
	}
}

func TestScoreTasksKeywordAndDuration(t *testing.T) {
	tasks := []Task{
		{Description: "a quick easy stretch", DurationMin: 10},
		{Description: "an intense public march", DurationMin: 60},
	}
	spec := RequestSpec{DurationMin: 10, Keywords: []string{"quick"}}
	scores := ScoreTasks(tasks, spec)
	if scores[0] <= scores[1] {
		t.Fatalf("scores=%v", scores)
	}
	if idx := PickByScore(scores); idx != 0 {
		t.Fatalf("idx=%d scores=%v", idx, scores)
	}
}

func TestPickByScoreEmpty(t *testing.T) {
	if idx := PickByScore([]int{0, 0}); idx != -1 {
		t.Fatalf("idx=%d", idx)
	}
}

func TestScoreTasksEmptySpec(t *testing.T) {
	tasks := []Task{{Description: "anything"}}
	if scores := ScoreTasks(tasks, RequestSpec{}); scores[0] != 0 {
		t.Fatalf("scores=%v", scores)
	}
}

func TestRephraseNilClient(t *testing.T) {
	if got := RephraseTask(nilContext(), nil, "x"); got != "x" {
		t.Fatalf("got %q", got)
	}
}

func nilContext() context.Context { return context.Background() }
