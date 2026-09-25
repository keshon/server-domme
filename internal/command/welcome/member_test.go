package welcome

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// A /welcome member as Discord delivers it. %s is the options of the
// subcommand.
const welcomeMember = `{
  "type": 2, "id": "i1", "application_id": "app", "token": "tok",
  "guild_id": "g1", "channel_id": "ticket-1",
  "member": {"user": {"id": "admin", "username": "admin"}, "roles": ["r-admin"], "permissions": "8"},
  "data": {"id": "cmd", "name": "welcome", "type": 1,
           "options": [{"name": "member", "type": 1, "options": [%s]}]}
}`

const introChannel, generalChannel = "100", "101"

// memberRun sets a role up with both parts and runs /welcome member with the
// given options.
func memberRun(t *testing.T, options string) *recorder {
	t.Helper()
	var ic discordgo.InteractionCreate
	if err := json.Unmarshal([]byte(strings.Replace(welcomeMember, "%s", options, 1)), &ic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	store := testStore(t)
	if err := store.UpdateWelcomeRole(guild, "r1", func(w *storage.WelcomeRole) {
		w.IntroChannel, w.IntroTemplate = introChannel, "meet {user}"
		w.WelcomeChannel, w.WelcomeTemplate = generalChannel, "welcome {user}"
	}); err != nil {
		t.Fatal(err)
	}

	rec := &recorder{}
	state := discordgo.NewState()
	if err := state.GuildAdd(&discordgo.Guild{ID: guild, Name: "Queen's Court",
		Roles:    []*discordgo.Role{{ID: "r1", Name: "dommes"}},
		Channels: []*discordgo.Channel{{ID: introChannel, GuildID: guild, Name: "introduction"}, {ID: generalChannel, GuildID: guild, Name: "general"}},
		Members:  []*discordgo.Member{member("u1", "r1")},
	}); err != nil {
		t.Fatal(err)
	}
	sess := &discordgo.Session{State: state, Client: &http.Client{Transport: rec}, Ratelimiter: discordgo.NewRatelimiter()}
	ctx := &cmdadapter.SlashInteractionContext{Session: sess, Event: &ic, Storage: store, Config: &config.Config{}, AppLog: zerolog.Nop()}
	if err := (&WelcomeCommand{}).Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	return rec
}

// postedIn reports whether a message went to a channel.
func (r *recorder) postedIn(channelID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.paths {
		if p == http.MethodPost+" /api/v9/channels/"+channelID+"/messages" {
			return true
		}
	}
	return false
}

// report is what the administrator was told, the last thing sent.
func (r *recorder) report() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.bodies) == 0 {
		return ""
	}
	return r.bodies[len(r.bodies)-1]
}

const userOpt = `{"name": "user", "type": 6, "value": "u1"}`

// The half that failed can be posted without posting the half that went
// out: an intro landed, the welcome was refused for a missing permission,
// and until now the only way to finish was to post both again.
func TestWelcomeMemberPostsOnlyThePartAskedFor(t *testing.T) {
	rec := memberRun(t, userOpt+`, {"name": "part", "type": 3, "value": "welcome"}`)
	if rec.postedIn(introChannel) {
		t.Error("the intro was posted again")
	}
	if !rec.postedIn(generalChannel) {
		t.Fatalf("the welcome did not go out: %v", rec.paths)
	}
	if got := rec.report(); !strings.Contains(got, "not this time") {
		t.Errorf("the report does not say the intro was left out: %s", got)
	}

	// The other way round.
	rec = memberRun(t, userOpt+`, {"name": "part", "type": 3, "value": "intro"}`)
	if !rec.postedIn(introChannel) || rec.postedIn(generalChannel) {
		t.Errorf("intro only posted %v", rec.paths)
	}
}

// Left empty it does what it always did, and posts both.
func TestWelcomeMemberStillPostsBothByDefault(t *testing.T) {
	rec := memberRun(t, userOpt)
	if !rec.postedIn(introChannel) || !rec.postedIn(generalChannel) {
		t.Errorf("posted %v", rec.paths)
	}
}
