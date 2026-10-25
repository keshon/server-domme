package welcome

import (
	"context"
	"strings"
	"testing"

	"github.com/keshon/command"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/middleware"
	"github.com/keshon/server-domme/internal/discord/perm"
	"github.com/keshon/server-domme/internal/storage"
)

func TestWelcomeAccessBoundary(t *testing.T) {
	store := testStore(t)
	if err := store.SetWelcomeOperator(guild, "greeter", true); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, guildID, role, sub string
		perms                    int64
		want                     bool
	}{
		{"member", guild, "greeter", subMember, 0, true},
		{"preview", guild, "greeter", subPreview, 0, true},
		{"other role", guild, "other", subMember, 0, false},
		{"other guild", "other", "greeter", subMember, 0, false},
		{"admin", guild, "other", subSetup, perm.Administrator, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testCtx(store, newAPIFake())
			ctx.Invoker.GuildID, ctx.Invoker.Roles, ctx.Invoker.Permissions = tc.guildID, []string{tc.role}, tc.perms
			got, err := canRun(ctx, tc.sub)
			if err != nil || got != tc.want {
				t.Fatalf("canRun = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	ctx := testCtx(store, newAPIFake())
	ctx.Invoker.Roles, ctx.Invoker.Permissions = []string{"greeter"}, 0
	// Test the actual dispatch path: every declared configuration action and
	// unknown future action must be refused before it can mutate or call APIs.
	wrapped := command.Apply(&adapter.Adapter{Cmd: &WelcomeCommand{}}, middleware.WithUserPermissionCheck())
	for _, sub := range []string{subSetup, subTemplate, subRoles, subMove, subRemove, subGifAdd, subGifRemove, subGifs, subAccess, "future"} {
		ctx.Arguments = []adapter.SlashArgument{{Type: adapter.OptionSubCommand, Name: sub}}
		if err := wrapped.Run(context.Background(), &command.Invocation{Data: ctx}); err != nil {
			t.Fatal(err)
		}
		if got := ctx.Responder.(*responderFake).lastReply(); !strings.Contains(got.Embed.Description, "Only administrators") {
			t.Fatalf("%s was not refused: %+v", sub, got)
		}
	}
	if err := store.SetWelcomeOperator(guild, "greeter", false); err != nil {
		t.Fatal(err)
	}
	if allowed, err := canRun(ctx, subMember); err != nil || allowed {
		t.Fatalf("revoked role allowed = %v, %v", allowed, err)
	}
	ctx.Config = &config.Config{DeveloperID: ctx.UserID()}
	if allowed, err := canRun(ctx, subSetup); err != nil || !allowed {
		t.Fatalf("developer allowed = %v, %v", allowed, err)
	}
	ctx.Config = nil
	ctx.Storage = nil
	if allowed, err := canRun(ctx, subMember); err != nil || allowed {
		t.Fatalf("missing store allowed = %v, %v", allowed, err)
	}
}

func TestWelcomeOperatorCanPostAndPreviewThroughMiddleware(t *testing.T) {
	store := testStore(t)
	if err := store.SetWelcomeOperator(guild, "greeter", true); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateWelcomeRole(guild, "recipient", func(w *storage.WelcomeRole) {
		w.IntroChannel, w.IntroTemplate = "intro", "Hello {user}"
	}); err != nil {
		t.Fatal(err)
	}
	api := newAPIFake()
	api.members["newcomer"] = member("newcomer", "recipient")
	ctx := testCtx(store, api)
	ctx.Invoker.Permissions, ctx.Invoker.Roles = 0, []string{"greeter"}
	wrapped := command.Apply(&adapter.Adapter{Cmd: &WelcomeCommand{}}, middleware.WithUserPermissionCheck())
	ctx.Arguments = []adapter.SlashArgument{{Type: adapter.OptionSubCommand, Name: subPreview, Options: []adapter.SlashArgument{roleOpt("recipient")}}}
	if err := wrapped.Run(context.Background(), &command.Invocation{Data: ctx}); err != nil {
		t.Fatal(err)
	}
	if len(api.posts) != 0 {
		t.Fatal("preview posted publicly")
	}
	if got := ctx.Responder.(*responderFake).lastReply(); !strings.Contains(got.Embed.Description, "Preview for") {
		t.Fatal("preview was refused")
	}
	ctx.Arguments = []adapter.SlashArgument{{Type: adapter.OptionSubCommand, Name: subMember, Options: []adapter.SlashArgument{{Name: optUser, Value: "newcomer"}}}}
	if err := wrapped.Run(context.Background(), &command.Invocation{Data: ctx}); err != nil {
		t.Fatal(err)
	}
	if len(api.posts) != 1 || api.posts[0].msg.Content != "Hello <@newcomer>" {
		t.Fatalf("posts = %+v", api.posts)
	}
}

func TestWelcomeAccessManagementAndModal(t *testing.T) {
	store := testStore(t)
	ctx := testCtx(store, newAPIFake())
	c := &WelcomeCommand{}
	args := []adapter.SlashArgument{roleOpt("greeter"), {Name: optAllow, Value: true}}
	ctx.Arguments = []adapter.SlashArgument{{Name: subAccess, Options: args}}
	if err := c.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := store.WelcomeOperators(guild); len(got) != 1 || got[0] != "greeter" {
		t.Fatalf("roles = %v", got)
	}
	ctx.Arguments[0].Options = nil
	if err := c.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if got := ctx.Responder.(*responderFake).lastReply(); !strings.Contains(got.Embed.Description, "<@&greeter>") {
		t.Fatal("list omitted operator")
	}
	resp, modal := modalCtx(t, newAPIFake(), store, "unauthorized template")
	modal.Invoker.Permissions, modal.Invoker.Roles = 0, []string{"greeter"}
	if err := c.ModalSubmit(modal); err != nil {
		t.Fatal(err)
	}
	if store.WelcomeRoleFor(guild, "r1") != nil {
		t.Fatal("operator saved a template")
	}
	if !strings.Contains(resp.lastReply().Embed.Description, "Only administrators") {
		t.Fatal("modal was not refused")
	}
	args[1].Value = false
	ctx.Arguments[0].Options = args
	if err := c.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(store.WelcomeOperators(guild)) != 0 {
		t.Fatal("revoked role remains")
	}
}
