package mind

import (
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/memory"
	"github.com/rs/zerolog"
)

// theFight is the 25 September log, in miniature: one thing said, then the
// same thing turned over again and again, each time as a fresh heavy
// memory, with a watch on the person for good measure.
func theFight(t *testing.T, m *Mind, at time.Time) {
	t.Helper()
	them := []memory.Ref{{ID: "123", Name: "Big M"}}
	for _, mo := range []memory.Moment{
		{At: at, Channel: "chat", People: them, Weight: 0.7, Text: "Big M told me to shut up when I called out his bypassing."},
		{At: at.Add(time.Hour), Channel: "chat", People: them, Weight: 0.7, Text: "He told me to shut up yesterday and hasn't acknowledged it."},
		{At: at.Add(2 * time.Hour), Channel: "chat", People: them, Weight: 0.6, Text: "Big M told me to shut up and today he is still not owning it."},
		{At: at.Add(2 * time.Hour), Channel: "chat", People: them, Weight: 0.8, Text: "His apology rings hollow given the shut-up line."},
	} {
		if err := m.Memory.AddMoment(guildID, mo); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Memory.AddThread(guildID, memory.Thread{
		Person: them[0], Text: "Watch whether he stops routing around me or keeps deflecting",
	}); err != nil {
		t.Fatal(err)
	}
}

// Taking an apology settles what it was about: the weight comes out of
// every heavy memory of it, so it fades like anything else, the watch is
// closed, and the making-up weighs what the falling-out did.
func TestMendingTakesTheWeightOutOfTheFight(t *testing.T) {
	m, _ := newMind(t)
	m.Log = zerolog.Nop()
	theFight(t, m, noon.Add(-3*time.Hour))

	s := sceneWith(him("that was out of line yesterday, I'm sorry", noon))
	a := Appraisal{Act: ActReply, Mended: true, Weight: 0.2, Toward: "softening",
		Remember: "He apologised for the shut-up line and meant it."}
	if err := m.Absorb(s, a); err != nil {
		t.Fatal(err)
	}

	day, _ := m.Memory.Day(guildID, noon)
	for _, mo := range day.Moments {
		if mo.At.Before(noon) && mo.Weight > memory.SettledWeight {
			t.Errorf("still heavy: %.1f %q", mo.Weight, mo.Text)
		}
	}
	// The apology itself is remembered as heavily as the worst of it.
	var made *memory.Moment
	for i, mo := range day.Moments {
		if mo.At.Equal(noon) {
			made = &day.Moments[i]
		}
	}
	if made == nil || made.Weight < 0.8 {
		t.Errorf("the mending weighs %+v", made)
	}
	open, err := m.Memory.Threads(guildID)
	if err != nil {
		t.Fatal(err)
	}
	if left := memory.Unfinished(open); len(left) != 0 {
		t.Errorf("still watching him: %+v", left)
	}
}

// Where things stand, written mid-fight, does not survive the fight being
// settled — unless she says where they stand now.
func TestMendingClearsWhatWasWrittenMidFight(t *testing.T) {
	for _, now := range []string{"", "we are fine, and he knows it"} {
		m, _ := newMind(t)
		m.Log = zerolog.Nop()
		s := sceneWith(him("sorry", noon))
		if err := m.Absorb(s, Appraisal{Act: ActReply, Weight: 0.7,
			Between: "my boundary is being actively violated"}); err != nil {
			t.Fatal(err)
		}
		if err := m.Absorb(s, Appraisal{Act: ActReply, Mended: true, Weight: 0.3, Between: now}); err != nil {
			t.Fatal(err)
		}
		p, _, _ := m.Memory.Person(guildID, "123")
		if p.Between != now {
			t.Errorf("between is %q, want %q", p.Between, now)
		}
	}
}

// A thing already written down is not written down again: brooding is not
// remembering.
func TestTheSameThingIsNotRememberedTwice(t *testing.T) {
	m, _ := newMind(t)
	m.Log = zerolog.Nop()
	s := sceneWith(him("whatever", noon))
	first := Appraisal{Act: ActReply, Weight: 0.7, Remember: "He told me to shut up when I called out his bypassing."}
	again := Appraisal{Act: ActReply, Weight: 0.7, Remember: "He told me to shut up after I called out the bypassing, and has not owned it."}
	for _, a := range []Appraisal{first, again} {
		if err := m.Absorb(s, a); err != nil {
			t.Fatal(err)
		}
	}
	day, _ := m.Memory.Day(guildID, noon)
	if len(day.Moments) != 1 {
		t.Errorf("wrote %d moments: %+v", len(day.Moments), day.Moments)
	}
}

// An hour of heavy moments with someone opens no new accounts on them.
func TestSheOpensNoNewWatchWhileTheHourIsGoingBadly(t *testing.T) {
	m, _ := newMind(t)
	m.Log = zerolog.Nop()
	theFight(t, m, noon.Add(-30*time.Minute))
	if _, err := m.Memory.CloseThreadsAbout(guildID, "123"); err != nil {
		t.Fatal(err)
	}

	s := sceneWith(him("I already said sorry", noon))
	if err := m.Absorb(s, Appraisal{Act: ActReply, Weight: 0.7,
		Later: "Watch whether he keeps deflecting or actually owns it"}); err != nil {
		t.Fatal(err)
	}
	open, _ := m.Memory.Threads(guildID)
	if left := memory.Unfinished(open); len(left) != 0 {
		t.Errorf("opened %+v while grinding", left)
	}
}

// A follow-up about somebody else is filed under them, and one already
// meant is not meant twice however was talking at the time.
func TestAFollowUpIsFiledUnderWhoItIsAbout(t *testing.T) {
	m, _ := newMind(t)
	m.Log = zerolog.Nop()
	if err := m.Memory.UpdatePerson(guildID, "999", func(p *memory.Person) { p.Name = "Duchess" }); err != nil {
		t.Fatal(err)
	}
	s := sceneWith(him("she is only trying to help", noon))
	if err := m.Absorb(s, Appraisal{Act: ActReply, Weight: 0.2,
		Later: "Watch whether Duchess respects my handling of this or keeps meddling"}); err != nil {
		t.Fatal(err)
	}
	open, _ := m.Memory.Threads(guildID)
	left := memory.Unfinished(open)
	if len(left) != 1 || left[0].Person.ID != "999" {
		t.Fatalf("filed %+v", left)
	}

	// The same suspicion, while somebody else is talking.
	other := sceneWith(Turn{UserID: "777", Username: "Pewcifer", Content: "bring it", At: noon})
	other.UserID, other.Username = "777", "Pewcifer"
	if err := m.Absorb(other, Appraisal{Act: ActReply, Weight: 0.2,
		Later: "Watch if Duchess respects my direct handling or keeps meddling"}); err != nil {
		t.Fatal(err)
	}
	open, _ = m.Memory.Threads(guildID)
	if left := memory.Unfinished(open); len(left) != 1 {
		t.Errorf("the same watch was opened twice: %+v", left)
	}
}
