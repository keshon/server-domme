package middleware

import (
	"context"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/command"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
)

const (
	testGuildID   = "g1"
	testChannelID = "c1"
	developerID   = "dev-1"
)

// adminOnly is a command nobody may run without the Administrator permission.
type adminOnly struct{ ran *bool }

func (a *adminOnly) Name() string        { return "admin-only" }
func (a *adminOnly) Description() string { return "requires administrator" }
func (a *adminOnly) Group() string       { return "test" }
func (a *adminOnly) Category() string    { return "test" }
func (a *adminOnly) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func (a *adminOnly) Run(ctx interface{}) error {
	*a.ran = true
	return nil
}

// testSession builds a session whose state already answers every lookup
// UserChannelPermissions makes, so the middleware resolves permissions locally
// and never reaches the REST client — which in a bare session panics.
func testSession(t *testing.T, userID string) *discordgo.Session {
	t.Helper()

	state := discordgo.NewState()
	state.User = &discordgo.User{ID: "bot-1"}

	guild := &discordgo.Guild{
		ID:      testGuildID,
		OwnerID: "someone-who-is-not-the-caller",
		// A single role carrying no permissions: @everyone with nothing
		// granted, which is what makes the caller an ordinary member.
		Roles: []*discordgo.Role{{ID: testGuildID, Permissions: 0}},
	}
	if err := state.GuildAdd(guild); err != nil {
		t.Fatalf("GuildAdd: %v", err)
	}
	if err := state.ChannelAdd(&discordgo.Channel{ID: testChannelID, GuildID: testGuildID}); err != nil {
		t.Fatalf("ChannelAdd: %v", err)
	}
	if err := state.MemberAdd(&discordgo.Member{
		GuildID: testGuildID,
		User:    &discordgo.User{ID: userID},
	}); err != nil {
		t.Fatalf("MemberAdd: %v", err)
	}

	return &discordgo.Session{State: state}
}

// slashInvocation builds a slash context with no Responder, so a refusal has
// nowhere to send its explanation and the test never reaches the REST client.
func slashInvocation(t *testing.T, cfg *config.Config, userID string) *command.Invocation {
	t.Helper()
	return &command.Invocation{Data: &cmdadapter.SlashInteractionContext{
		Session: testSession(t, userID),
		Event: &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{
			GuildID:   testGuildID,
			ChannelID: testChannelID,
			Member:    &discordgo.Member{GuildID: testGuildID, User: &discordgo.User{ID: userID}},
		}},
		Config: cfg,
	}}
}

func runAdminOnly(t *testing.T, cfg *config.Config, userID string) (ran bool, err error) {
	t.Helper()
	cmd := command.Apply(&cmdadapter.Adapter{Cmd: &adminOnly{ran: &ran}}, WithUserPermissionCheck())
	err = cmd.Run(context.Background(), slashInvocation(t, cfg, userID))
	return ran, err
}

// DEVELOPER_ID exists so the maintainer can exercise admin commands on a live
// server without holding a role there, or waking an admin in another timezone
// to grant one. perm.IsAdministrator honoured it before this middleware did,
// which meant the gate that actually refuses commands did not.
func TestDeveloperBypassesTheUserPermissionGate(t *testing.T) {
	ran, err := runAdminOnly(t, &config.Config{DeveloperID: developerID}, developerID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !ran {
		t.Error("the developer was refused a command they are meant to run anywhere")
	}
}

// The control. Without it the test above would pass just as happily on a
// middleware that lets everyone through.
func TestNonDeveloperDoesNotBypassTheUserPermissionGate(t *testing.T) {
	ran, err := runAdminOnly(t, &config.Config{DeveloperID: developerID}, "someone-else")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ran {
		t.Error("an ordinary member ran an administrator-only command")
	}
}

// An unset DEVELOPER_ID must not match an absent user id, or a malformed event
// would arrive holding the keys to every guild the bot is in.
func TestEmptyDeveloperIDMatchesNobody(t *testing.T) {
	ran, err := runAdminOnly(t, &config.Config{}, "")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ran {
		t.Error("an empty user id matched an unset DEVELOPER_ID and bypassed the gate")
	}
}

// A nil config must refuse rather than bypass: it is what a context that was
// never given one looks like, and failing open there would be silent.
func TestNilConfigDoesNotBypass(t *testing.T) {
	ran, err := runAdminOnly(t, nil, developerID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ran {
		t.Error("a missing config bypassed the permission gate")
	}
}
