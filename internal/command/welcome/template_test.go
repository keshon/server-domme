package welcome

import "testing"

var channels = []Channel{
	{ID: "1", Name: "introduction"},
	{ID: "2", Name: "roles"},
	{ID: "3", Name: "😍-kinks"},
	{ID: "4", Name: "roles-info"},
	{ID: "5", Name: "subs-chat"},
}

// The template as Duchess wrote it, pasted from Discord: channels arrive as
// "#name" and have to become links again.
func TestRenderLinksPastedChannels(t *testing.T) {
	got := Render(
		"{user} Please fill #introduction and grab any #roles you like. Also we have a #😍-kinks channel, ask in #subs-chat!",
		Vars{UserID: "42"}, channels)
	want := "<@42> Please fill <#1> and grab any <#2> you like. Also we have a <#3> channel, ask in <#5>!"
	if got != want {
		t.Errorf("Render =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderLeavesWhatItCannotLink(t *testing.T) {
	cases := map[string]string{
		"see #roles-info":      "see <#4>",
		"see #roleplay":        "see #roleplay",
		"already <#2> linked":  "already <#2> linked",
		"#1 fan":               "#1 fan",
		"welcome to {server}!": "welcome to The Parlour!",
		"you are a {role} now": "you are a sub now",
	}
	for in, want := range cases {
		if got := Render(in, Vars{UserID: "42", Server: "The Parlour", Role: "sub"}, channels); got != want {
			t.Errorf("Render(%q) = %q, want %q", in, got, want)
		}
	}
}
