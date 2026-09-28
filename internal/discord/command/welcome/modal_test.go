package welcome

import (
	"strings"
	"testing"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/perm"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

func modalCtx(t *testing.T, api *apiFake, store *storage.Storage, text string) (*responderFake, *adapter.ModalSubmitContext) {
	t.Helper()
	resp := &responderFake{}
	ctx := &adapter.ModalSubmitContext{
		Invoker: adapter.Invoker{
			GuildID: guild, ChannelID: "c1",
			UserID: "admin", Username: "admin", DisplayName: "admin",
			Permissions: perm.Administrator, PermissionsKnown: true,
		},
		Responder:   resp,
		API:         api,
		ComponentID: "welcome:tpl:intro:r1",
		Values:      map[string]string{modalField: text},
		Storage:     store,
		AppLog:      zerolog.Nop(),
	}
	return resp, ctx
}

func TestAnAdministratorCanSaveATemplateFromTheEditor(t *testing.T) {
	api := newAPIFake()
	api.channels = []adapter.Channel{
		{ID: "100", Name: "introduction"},
		{ID: "101", Name: "😍-kinks"},
	}
	api.guild = adapter.GuildInfo{ID: guild, Name: "Queen's Court"}
	store := testStore(t)
	resp, ctx := modalCtx(t, api, store, "Please fill #introduction and see #😍-kinks")
	if err := (&WelcomeCommand{}).ModalSubmit(ctx); err != nil {
		t.Fatalf("ModalSubmit: %v", err)
	}
	w := store.WelcomeRoleFor(guild, "r1")
	if w == nil || w.IntroTemplate != "Please fill #introduction and see #😍-kinks" {
		t.Fatalf("saved %+v", w)
	}
	// Acknowledged first, then answered: the save may look things up, and
	// Discord gives a modal only three seconds.
	if len(resp.actions) != 2 || resp.actions[0] != "defer:true" || resp.actions[1] != "followup" {
		t.Fatalf("answered %v", resp.actions)
	}
	got := resp.lastReply()
	if got.Embed == nil || !strings.Contains(got.Embed.Description, "Please fill <#100> and see <#101>") {
		t.Errorf("the preview reads %+v", got.Embed)
	}
}

// Copied out of a message rather than typed, a channel can arrive as the
// "<#id>" Discord itself uses. That is already a link and has to stay one.
func TestAChannelInDiscordsOwnFormatStaysALink(t *testing.T) {
	channels := []adapter.Channel{{ID: "100", Name: "introduction"}}
	got := Render("Please fill <#100> and #introduction, not <#999>", Vars{}, channels)
	if got != "Please fill <#100> and <#100>, not <#999>" {
		t.Errorf("rendered %q", got)
	}
	if missing := unlinked(got); len(missing) != 0 {
		t.Errorf("a linked channel was reported missing: %v", missing)
	}
}
