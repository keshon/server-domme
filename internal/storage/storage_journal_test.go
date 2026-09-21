package storage

import "testing"

func TestJournalKeepsTheNewestPerChannel(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < mindJournalLimit+5; i++ {
		if _, err := s.AddMindJournal(MindJournal{GuildID: "g", ChannelID: "c1", Trigger: "mention", Outcome: "answered"}); err != nil {
			t.Fatalf("AddMindJournal: %v", err)
		}
	}
	if _, err := s.AddMindJournal(MindJournal{GuildID: "g", ChannelID: "c2", Trigger: "mention"}); err != nil {
		t.Fatalf("AddMindJournal: %v", err)
	}

	got := s.MindJournalIn("g", "c1")
	if len(got) > mindJournalLimit {
		t.Errorf("channel holds %d entries, want at most %d", len(got), mindJournalLimit)
	}
	if got[len(got)-1].ID <= got[0].ID {
		t.Error("entries are not oldest first")
	}
	if len(s.MindJournalIn("g", "c2")) != 1 {
		t.Error("one channel's trimming reached another's")
	}
}

func TestJournalEntryIsUpdatedInPlace(t *testing.T) {
	s := newTestStore(t)
	id, err := s.AddMindJournal(MindJournal{GuildID: "g", ChannelID: "c", Outcome: "queued"})
	if err != nil {
		t.Fatalf("AddMindJournal: %v", err)
	}
	if err := s.UpdateMindJournal("g", id, func(j *MindJournal) { j.Outcome, j.Posted = "answered", "hey" }); err != nil {
		t.Fatalf("UpdateMindJournal: %v", err)
	}
	if got := s.MindJournalIn("g", "c"); got[0].Outcome != "answered" || got[0].Posted != "hey" {
		t.Errorf("entry after update: %+v", got[0])
	}
	if err := s.UpdateMindJournal("g", 999, func(*MindJournal) {}); err != nil {
		t.Errorf("updating a trimmed entry failed: %v", err)
	}
}

// It carries excerpts of what people said, so it goes with the memories.
func TestForgetClearsTheJournal(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AddMindJournal(MindJournal{GuildID: "g", ChannelID: "c", Excerpt: "something private"}); err != nil {
		t.Fatalf("AddMindJournal: %v", err)
	}
	if err := s.ForgetMind("g"); err != nil {
		t.Fatalf("ForgetMind: %v", err)
	}
	if got := s.MindJournalIn("g", "c"); len(got) != 0 {
		t.Errorf("journal survived forget: %+v", got)
	}
}

func TestDayCountsAccumulatePerDay(t *testing.T) {
	s := newTestStore(t)
	for _, e := range []string{"answered", "answered", "silent"} {
		if err := s.CountMindEvent("g", "2026-09-18", e); err != nil {
			t.Fatalf("CountMindEvent: %v", err)
		}
	}
	got := s.MindDayCounts("g", "2026-09-18")
	if got["answered"] != 2 || got["silent"] != 1 {
		t.Errorf("counts = %v", got)
	}
	if len(s.MindDayCounts("g", "2026-09-19")) != 0 {
		t.Error("a new day started with counts")
	}
}
