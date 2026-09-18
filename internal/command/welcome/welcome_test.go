package welcome

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

const guild = "g1"

func testStore(t *testing.T) *storage.Storage {
	t.Helper()
	s, err := storage.NewStorage(t.TempDir(), zerolog.Nop())
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func member(id string, roles ...string) *discordgo.Member {
	return &discordgo.Member{GuildID: guild, User: &discordgo.User{ID: id}, Roles: roles}
}

func roleOpt(id string) *discordgo.ApplicationCommandInteractionDataOption {
	return &discordgo.ApplicationCommandInteractionDataOption{Name: optRole, Type: discordgo.ApplicationCommandOptionRole, Value: id}
}

func setUp(t *testing.T, store *storage.Storage, roles ...string) {
	t.Helper()
	for _, r := range roles {
		if err := store.UpdateWelcomeRole(guild, r, func(w *storage.WelcomeRole) { w.IntroChannel = "c-" + r }); err != nil {
			t.Fatalf("UpdateWelcomeRole: %v", err)
		}
	}
}

// Welcoming someone as a role they do not have is the mistake this command
// most needs to refuse.
func TestPickRoleRefusesARoleTheyDoNotHave(t *testing.T) {
	store := testStore(t)
	setUp(t, store, "sub", "domme")

	if _, problem := pickRole(store, guild, member("u1", "sub"), roleOpt("domme")); problem == "" {
		t.Error("welcomed a sub as a Domme")
	}
	if got, problem := pickRole(store, guild, member("u1", "sub"), roleOpt("sub")); problem != "" || got != "sub" {
		t.Errorf("refused the role they have: %q", problem)
	}
}

func TestPickRoleWorksOutTheirRoleOrAsks(t *testing.T) {
	store := testStore(t)

	if _, problem := pickRole(store, guild, member("u1", "sub"), nil); !strings.Contains(problem, "/welcome setup") {
		t.Errorf("with nothing set up: %q", problem)
	}

	setUp(t, store, "sub", "domme")
	if got, problem := pickRole(store, guild, member("u1", "sub", "unrelated"), nil); problem != "" || got != "sub" {
		t.Errorf("one configured role: got %q, %q", got, problem)
	}
	if _, problem := pickRole(store, guild, member("u1", "sub", "domme"), nil); !strings.Contains(problem, "role:") {
		t.Errorf("two configured roles did not ask which: %q", problem)
	}
	if _, problem := pickRole(store, guild, member("u1", "unrelated"), nil); !strings.Contains(problem, "Give them one first") {
		t.Errorf("no configured role: %q", problem)
	}
	if _, problem := pickRole(store, guild, member("u1", "vip"), roleOpt("vip")); !strings.Contains(problem, "no welcome set up") {
		t.Errorf("a role with no welcome: %q", problem)
	}
}

// A session whose cache knows one text channel the bot may post in.
func stateWith(channelID string) *discordgo.Session {
	st := discordgo.NewState()
	_ = st.GuildAdd(&discordgo.Guild{ID: guild, Name: "The Parlour"})
	_ = st.ChannelAdd(&discordgo.Channel{ID: channelID, GuildID: guild, Name: "mousey", Type: discordgo.ChannelTypeGuildText})
	return &discordgo.Session{State: st}
}

func TestPlanPartChecksBeforeAnythingIsPosted(t *testing.T) {
	s := stateWith("c1")
	v := Vars{UserID: "u1"}

	if p := planPart(s, guild, "Intro", "", "", v, nil, ""); p.ok() || p.skip == "" {
		t.Errorf("an unset part was not skipped: %+v", p)
	}
	if p := planPart(s, guild, "Intro", "c1", "", v, nil, ""); p.ok() || !strings.Contains(p.problem, "/welcome template") {
		t.Errorf("a part with no text: %+v", p)
	}
	if p := planPart(s, guild, "Intro", "", "hi {user}", v, nil, ""); p.ok() || !strings.Contains(p.problem, "/welcome setup") {
		t.Errorf("a part with no channel: %+v", p)
	}
	if p := planPart(s, guild, "Intro", "c1", strings.Repeat("x", 2001), v, nil, ""); p.ok() {
		t.Error("a text over Discord's limit was allowed")
	}
	p := planPart(s, guild, "Intro", "c1", "hi {user}", v, nil, "")
	if !p.ok() || p.content != "hi <@u1>" {
		t.Errorf("a good part: %+v", p)
	}
}

func TestUnlinkedFlagsChannelNamesThatMatchNothing(t *testing.T) {
	got := unlinked("see <#1> and #roleplay, then #1 fan. also# not")
	if len(got) != 2 || got[0] != "#roleplay" || got[1] != "#1" {
		t.Errorf("unlinked = %v", got)
	}
}

func TestModalValueReadsTheSubmittedText(t *testing.T) {
	components := []discordgo.MessageComponent{
		&discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			&discordgo.TextInput{CustomID: modalField, Value: "WELCOME {user}"},
		}},
	}
	if got := modalValue(components, modalField); got != "WELCOME {user}" {
		t.Errorf("modalValue = %q", got)
	}
}

// Every subcommand offered to Discord has to be one Run handles; an
// unhandled one answers "Unknown subcommand" to an administrator.
func TestEverySubcommandIsHandled(t *testing.T) {
	handled := map[string]bool{
		subMember: true, subSetup: true, subTemplate: true, subPreview: true,
		subRoles: true, subRemove: true, subGifAdd: true, subGifRemove: true, subGifs: true,
	}
	for _, o := range (&WelcomeCommand{}).SlashDefinition().Options {
		if !handled[o.Name] {
			t.Errorf("subcommand %q is offered but not handled", o.Name)
		}
		if len(o.Description) > 100 {
			t.Errorf("subcommand %q description is %d characters; Discord allows 100", o.Name, len(o.Description))
		}
		for _, inner := range o.Options {
			if len(inner.Description) > 100 {
				t.Errorf("%s %s description is %d characters; Discord allows 100", o.Name, inner.Name, len(inner.Description))
			}
		}
	}
}
