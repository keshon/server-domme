package welcome

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// A mention pasted from Discord carries an invisible word joiner before the
// "#". It is dropped, so the name links, or is flagged when it matches
// nothing.
func TestInvisibleCharactersFromDiscordAreDropped(t *testing.T) {
	chans := []Channel{{ID: "1", Name: "information"}}
	if got := Render("in ⁠#information and", Vars{}, chans); got != "in <#1> and" {
		t.Errorf("rendered %q", got)
	}
	if got := unlinked(Render("list here: ⁠#Domme Icons Full List .", Vars{}, chans)); len(got) != 1 {
		t.Errorf("an unmatched pasted name was not flagged: %v", got)
	}
	// Emoji built with a zero-width joiner are left whole.
	if got := Render("👩‍💻 #information", Vars{}, chans); got != "👩‍💻 <#1>" {
		t.Errorf("rendered %q", got)
	}
}

// archive is a fake Discord that answers every channel's archived threads
// with the same list.
type archive struct{ calls int }

func (a *archive) RoundTrip(r *http.Request) (*http.Response, error) {
	a.calls++
	body := `{"threads":[{"id":"7","name":"Domme Icons Full List","type":11},{"id":"9","name":"information","type":11}],"members":[],"has_more":false}`
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

func archivedSession(t *testing.T, a *archive) *discordgo.Session {
	t.Helper()
	s := &discordgo.Session{State: discordgo.NewState(), Client: &http.Client{Transport: a}, Ratelimiter: discordgo.NewRatelimiter()}
	if err := s.State.GuildAdd(&discordgo.Guild{ID: "g", Channels: []*discordgo.Channel{
		{ID: "1", GuildID: "g", Name: "information", Type: discordgo.ChannelTypeGuildText},
		{ID: "2", GuildID: "g", Name: "voice", Type: discordgo.ChannelTypeGuildVoice},
	}}); err != nil {
		t.Fatal(err)
	}
	return s
}

// Saved, a name only an archived thread has is written in as a link. A name
// an open channel has stays the channel's, and placeholders stay as they are.
func TestSavingLinksArchivedThreads(t *testing.T) {
	a := &archive{}
	s := archivedSession(t, a)
	text := "{user} see ⁠#information and the list here: #Domme Icons Full List . Welcome to {server}"
	got := linkArchived(s, "g", text, guildChannels(s, "g"))
	want := "{user} see #information and the list here: <#7> . Welcome to {server}"
	if got != want {
		t.Errorf("saved\n%q\nwant\n%q", got, want)
	}
	if a.calls != 1 {
		t.Errorf("%d requests, want one for the one text channel", a.calls)
	}
}

// When everything already links, nothing is looked up.
func TestSavingLooksNothingUpWhenAllLinks(t *testing.T) {
	a := &archive{}
	s := archivedSession(t, a)
	text := "see #information"
	if got := linkArchived(s, "g", text, guildChannels(s, "g")); got != text || a.calls != 0 {
		t.Errorf("saved %q after %d requests", got, a.calls)
	}
}
