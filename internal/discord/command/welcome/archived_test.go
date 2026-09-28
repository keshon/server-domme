package welcome

import (
	"testing"

	"github.com/keshon/server-domme/internal/discord/adapter"
)

// A mention pasted from Discord carries an invisible word joiner before the
// "#". It is dropped, so the name links, or is flagged when it matches
// nothing.
func TestInvisibleCharactersFromDiscordAreDropped(t *testing.T) {
	chans := []adapter.Channel{{ID: "1", Name: "information"}}
	if got := Render("in \u2060#information and", Vars{}, chans); got != "in <#1> and" {
		t.Errorf("rendered %q", got)
	}
	if got := unlinked(Render("list here: \u2060#Domme Icons Full List .", Vars{}, chans)); len(got) != 1 {
		t.Errorf("an unmatched pasted name was not flagged: %v", got)
	}
	// Emoji built with a zero-width joiner are left whole.
	if got := Render("👩‍💻 #information", Vars{}, chans); got != "👩‍💻 <#1>" {
		t.Errorf("rendered %q", got)
	}
}

// Saved, a name only an archived thread has is written in as a link. A name
// an open channel has stays the channel's, and placeholders stay as they are.
func TestSavingLinksArchivedThreads(t *testing.T) {
	api := newAPIFake()
	api.channels = []adapter.Channel{{ID: "1", Name: "information"}}
	api.archived = []adapter.Channel{
		{ID: "7", Name: "Domme Icons Full List"},
		{ID: "9", Name: "information"},
	}
	ctx := testCtx(testStore(t), api)
	text := "{user} see \u2060#information and the list here: #Domme Icons Full List . Welcome to {server}"
	got := linkArchived(ctx, text, api.channels)
	want := "{user} see #information and the list here: <#7> . Welcome to {server}"
	if got != want {
		t.Errorf("saved\n%q\nwant\n%q", got, want)
	}
	if api.archivedCalls != 1 {
		t.Errorf("%d lookups, want one", api.archivedCalls)
	}
}

// When everything already links, nothing is looked up.
func TestSavingLooksNothingUpWhenAllLinks(t *testing.T) {
	api := newAPIFake()
	api.channels = []adapter.Channel{{ID: "1", Name: "information"}}
	ctx := testCtx(testStore(t), api)
	text := "see #information"
	if got := linkArchived(ctx, text, api.channels); got != text || api.archivedCalls != 0 {
		t.Errorf("saved %q after %d lookups", got, api.archivedCalls)
	}
}

// Threads are kept apart from channels in the guild's listing; a template
// can name either.
func TestGuildChannelsLinkThreadsWithSpaces(t *testing.T) {
	channels := []adapter.Channel{
		{ID: "1", Name: "introduction"},
		{ID: "7", Name: "Domme Icons Full List"},
	}
	got := Render("see #introduction and #Domme Icons Full List", Vars{}, channels)
	if got != "see <#1> and <#7>" {
		t.Errorf("rendered %q", got)
	}
}
