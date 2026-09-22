package welcome

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
)

// recorder stands in for Discord's API and keeps what was sent to it.
type recorder struct {
	mu     sync.Mutex
	bodies []string
	paths  []string
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}
	r.mu.Lock()
	r.bodies = append(r.bodies, string(body))
	r.paths = append(r.paths, req.Method+" "+req.URL.Path)
	r.mu.Unlock()
	if req.Method == http.MethodPatch {
		// Editing a response answers with the message, as Discord does.
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"m1"}`)),
			Header: http.Header{"Content-Type": []string{"application/json"}}, Request: req}, nil
	}
	return &http.Response{StatusCode: http.StatusNoContent, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: req}, nil
}

// submission is a modal submit exactly as Discord delivers one. Note what the
// member object lacks: an interaction's member carries no guild_id, and its
// resolved permissions come as a string.
const submission = `{
  "type": 5, "id": "i1", "application_id": "app", "token": "tok",
  "guild_id": "g1", "channel_id": "c1",
  "member": {"user": {"id": "admin", "username": "admin"}, "roles": ["r-admin"], "permissions": "8"},
  "data": {"custom_id": "welcome:tpl:intro:r1",
           "components": [{"type": 1, "components": [{"type": 4, "custom_id": "template", "value": "%s"}]}]}
}`

func submit(t *testing.T, text string) (*recorder, *WelcomeCommand, *cmdadapter.ComponentInteractionContext) {
	t.Helper()
	var ic discordgo.InteractionCreate
	raw := strings.Replace(submission, "%s", text, 1)
	if err := json.Unmarshal([]byte(raw), &ic); err != nil {
		t.Fatalf("unmarshal submission: %v", err)
	}
	rec := &recorder{}
	state := discordgo.NewState()
	if err := state.GuildAdd(&discordgo.Guild{ID: guild, Name: "Queen's Court", OwnerID: "owner",
		Roles: []*discordgo.Role{{ID: "r1", Name: "test sub"}, {ID: "r-admin", Name: "mods", Permissions: discordgo.PermissionAdministrator}},
		Channels: []*discordgo.Channel{
			{ID: "100", GuildID: guild, Name: "introduction"},
			{ID: "101", GuildID: guild, Name: "😍-kinks"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	sess := &discordgo.Session{State: state, Client: &http.Client{Transport: rec}, Ratelimiter: discordgo.NewRatelimiter()}
	return rec, &WelcomeCommand{}, &cmdadapter.ComponentInteractionContext{
		Session: sess, Event: &ic, Storage: testStore(t), Config: &config.Config{},
	}
}

func TestAnAdministratorCanSaveATemplateFromTheEditor(t *testing.T) {
	rec, cmd, ctx := submit(t, "Please fill #introduction and see #😍-kinks")
	if err := cmd.ModalSubmit(ctx); err != nil {
		t.Fatalf("ModalSubmit: %v", err)
	}
	w := ctx.Storage.WelcomeRoleFor(guild, "r1")
	if w == nil || w.IntroTemplate != "Please fill #introduction and see #😍-kinks" {
		t.Fatalf("saved %+v; sent %v", w, rec.bodies)
	}
	// Acknowledged first, then answered by editing that in: the save may look
	// things up, and Discord gives a modal only three seconds.
	if len(rec.bodies) != 2 || !strings.HasPrefix(rec.paths[0], "POST") || !strings.HasPrefix(rec.paths[1], "PATCH") {
		t.Fatalf("answered %v %v", rec.paths, rec.bodies)
	}
	var ack struct {
		Type int `json:"type"`
	}
	if err := json.Unmarshal([]byte(rec.bodies[0]), &ack); err != nil || ack.Type != int(discordgo.InteractionResponseDeferredChannelMessageWithSource) {
		t.Fatalf("acknowledged with %s: %v", rec.bodies[0], err)
	}
	var answer struct {
		Embeds []struct{ Description string } `json:"embeds"`
	}
	if err := json.Unmarshal([]byte(rec.bodies[1]), &answer); err != nil || len(answer.Embeds) != 1 {
		t.Fatalf("answer %s: %v", rec.bodies[1], err)
	}
	if got := answer.Embeds[0].Description; !strings.Contains(got, "Please fill <#100> and see <#101>") {
		t.Errorf("the preview reads %q", got)
	}
}

// Copied out of a message rather than typed, a channel can arrive as the
// "<#id>" Discord itself uses. That is already a link and has to stay one.
func TestAChannelInDiscordsOwnFormatStaysALink(t *testing.T) {
	channels := []Channel{{ID: "100", Name: "introduction"}}
	got := Render("Please fill <#100> and #introduction, not <#999>", Vars{}, channels)
	if got != "Please fill <#100> and <#100>, not <#999>" {
		t.Errorf("rendered %q", got)
	}
	if missing := unlinked(got); len(missing) != 0 {
		t.Errorf("a linked channel was reported missing: %v", missing)
	}
}
