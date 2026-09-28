package welcome

import (
	"strings"
	"testing"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/storage"
)

const introChannel, generalChannel = "100", "101"

// memberRun sets a role up with both parts and runs /welcome member with the
// given options.
func memberRun(t *testing.T, args []adapter.SlashArgument, setup func(*storage.WelcomeRole)) (*apiFake, *responderFake) {
	t.Helper()
	store := testStore(t)
	if err := store.UpdateWelcomeRole(guild, "r1", func(w *storage.WelcomeRole) {
		w.IntroChannel, w.IntroTemplate = introChannel, "meet {user}"
		w.WelcomeChannel, w.WelcomeTemplate = generalChannel, "welcome {user}"
		if setup != nil {
			setup(w)
		}
	}); err != nil {
		t.Fatal(err)
	}

	api := newAPIFake()
	api.members["u1"] = &adapter.Member{UserID: "u1", Username: "u1", Roles: []string{"r1"}}
	api.channels = []adapter.Channel{
		{ID: introChannel, Name: "introduction"},
		{ID: generalChannel, Name: "general"},
	}
	api.guild = adapter.GuildInfo{ID: guild, Name: "Queen's Court"}
	api.roles["r1"] = "dommes"

	resp := &responderFake{}
	ctx := testCtx(store, api)
	ctx.Responder = resp
	ctx.Arguments = []adapter.SlashArgument{{
		Name: subMember, Type: adapter.OptionSubCommand, Options: args,
	}}
	if err := (&WelcomeCommand{}).Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	return api, resp
}

func userArg() adapter.SlashArgument {
	return adapter.SlashArgument{Name: optUser, Type: adapter.OptionUser, Value: "u1"}
}

func strArg(name, value string) adapter.SlashArgument {
	return adapter.SlashArgument{Name: name, Type: adapter.OptionString, Value: value}
}

// The half that failed can be posted without posting the half that went
// out.
func TestWelcomeMemberPostsOnlyThePartAskedFor(t *testing.T) {
	api, _ := memberRun(t, []adapter.SlashArgument{userArg(), strArg(optPart, "welcome")}, nil)
	if api.postedIn(introChannel) {
		t.Error("the intro was posted again")
	}
	if !api.postedIn(generalChannel) {
		t.Fatal("the welcome did not go out")
	}

	// The other way round.
	api, _ = memberRun(t, []adapter.SlashArgument{userArg(), strArg(optPart, "intro")}, nil)
	if !api.postedIn(introChannel) || api.postedIn(generalChannel) {
		t.Error("intro only did not post just the intro")
	}
}

// Left empty it does what it always did, and posts both.
func TestWelcomeMemberStillPostsBothByDefault(t *testing.T) {
	api, _ := memberRun(t, []adapter.SlashArgument{userArg()}, nil)
	if !api.postedIn(introChannel) || !api.postedIn(generalChannel) {
		t.Error("both parts did not go out")
	}
}

// With welcome_notify on, @everyone in the text is allowed through.
func TestWelcomeMemberPingsEveryoneWhenNotifyEnabled(t *testing.T) {
	api, _ := memberRun(t, []adapter.SlashArgument{userArg()}, func(w *storage.WelcomeRole) {
		w.WelcomeNotifyAll = true
		w.WelcomeTemplate = "@everyone welcome {user}"
	})
	if !api.postedIn(generalChannel) {
		t.Fatal("the welcome did not go out")
	}
	post := api.lastPost()
	if !post.msg.AllowEveryone {
		t.Errorf("allowed mentions did not include everyone: %+v", post.msg)
	}
	if !strings.Contains(post.msg.Content, "@everyone") {
		t.Errorf("the text lost its @everyone: %q", post.msg.Content)
	}
}
