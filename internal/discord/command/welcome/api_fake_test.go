package welcome

import (
	"fmt"
	"sync"
	"time"

	"github.com/keshon/server-domme/internal/discord/adapter"
)

// apiFake stands in for the connection: scripted answers, recorded posts.
type apiFake struct {
	mu sync.Mutex

	members  map[string]*adapter.Member
	channels []adapter.Channel
	archived []adapter.Channel
	roles    map[string]string
	guild    adapter.GuildInfo

	canPost map[string]string
	canPing map[string]string
	perms   int64

	posts []postedMessage

	archivedCalls int

	// postErrs queues PostMessage failures, one per call. A test that
	// refuses the first upload scripts Discord's 50013 here.
	postErrs []error
}

type postedMessage struct {
	channelID string
	msg       adapter.OutgoingMessage
	id        string
}

func newAPIFake() *apiFake {
	return &apiFake{
		members:  make(map[string]*adapter.Member),
		roles:    make(map[string]string),
		canPost:  make(map[string]string),
		canPing:  make(map[string]string),
		guild:    adapter.GuildInfo{ID: "g1", Name: "Queen's Court"},
	}
}

func (f *apiFake) MemberPermissions(_, _ string) (int64, error) { return f.perms, nil }
func (f *apiFake) CheckBotPermissions(_ string) bool            { return true }
func (f *apiFake) SendChannelMessage(_, _ string) error         { return nil }
func (f *apiFake) SendChannelReply(_, _, _ string) error        { return nil }
func (f *apiFake) ClearChannelComponents(_, _ string) error     { return nil }
func (f *apiFake) SendChannelEmbed(_ string, _ *adapter.Embed) error {
	return nil
}
func (f *apiFake) SendDirectMessage(_, _ string) error { return nil }
func (f *apiFake) ChannelMessage(_, _ string) (*adapter.Message, error) {
	return &adapter.Message{}, nil
}
func (f *apiFake) ForwardMessage(_ string, _ *adapter.Message) error { return nil }
func (f *apiFake) RemoveReaction(_, _, _, _ string) error            { return nil }
func (f *apiFake) GuildMembers(_ string) ([]adapter.GuildMember, error) {
	return nil, nil
}
func (f *apiFake) AddMemberRole(_, _, _ string) error    { return nil }
func (f *apiFake) RemoveMemberRole(_, _, _ string) error { return nil }
func (f *apiFake) RoleName(_, roleID string) (string, error) {
	if name, ok := f.roles[roleID]; ok {
		return name, nil
	}
	return "", fmt.Errorf("welcome-test: no such role %s", roleID)
}
func (f *apiFake) RoleNames(_ string) (map[string]string, error) {
	out := make(map[string]string, len(f.roles))
	for id, name := range f.roles {
		out[id] = name
	}
	return out, nil
}
func (f *apiFake) ChannelMessages(_, _ string, _ int) ([]adapter.ListedMessage, error) {
	return nil, nil
}
func (f *apiFake) DeleteMessage(_, _ string) error { return nil }
func (f *apiFake) GuildInfo(_ string) (adapter.GuildInfo, error) {
	return f.guild, nil
}
func (f *apiFake) Latency() time.Duration { return 0 }
func (f *apiFake) EmbedColor() int        { return 0 }

func (f *apiFake) Member(_, userID string) (*adapter.Member, error) {
	if m, ok := f.members[userID]; ok {
		return m, nil
	}
	return nil, fmt.Errorf("welcome-test: no such member %s", userID)
}

func (f *apiFake) GuildChannels(_ string) ([]adapter.Channel, error) {
	return f.channels, nil
}

func (f *apiFake) GuildArchivedThreads(_ string) ([]adapter.Channel, error) {
	f.mu.Lock()
	f.archivedCalls++
	f.mu.Unlock()
	return f.archived, nil
}

func (f *apiFake) PostMessage(channelID string, msg adapter.OutgoingMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.postErrs) > 0 {
		err := f.postErrs[0]
		f.postErrs = f.postErrs[1:]
		return "", err
	}
	id := fmt.Sprintf("m%d", len(f.posts)+1)
	f.posts = append(f.posts, postedMessage{channelID: channelID, msg: msg, id: id})
	return id, nil
}

func (f *apiFake) CanPostIn(channelID, _ string) string {
	if why, ok := f.canPost[channelID]; ok {
		return why
	}
	return ""
}

func (f *apiFake) CanMentionEveryone(channelID string) string {
	if why, ok := f.canPing[channelID]; ok {
		return why
	}
	return ""
}

func (f *apiFake) postedIn(channelID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.posts {
		if p.channelID == channelID {
			return true
		}
	}
	return false
}

func (f *apiFake) lastPost() postedMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.posts) == 0 {
		return postedMessage{}
	}
	return f.posts[len(f.posts)-1]
}

// responderFake records an interaction's answers in order.
type responderFake struct {
	mu      sync.Mutex
	actions []string
	replies []adapter.Reply
}

func (r *responderFake) AckDeferred(ephemeral bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = append(r.actions, fmt.Sprintf("defer:%v", ephemeral))
	return nil
}

func (r *responderFake) Respond(rep adapter.Reply) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = append(r.actions, "respond")
	r.replies = append(r.replies, rep)
	return nil
}

func (r *responderFake) Followup(rep adapter.Reply) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = append(r.actions, "followup")
	r.replies = append(r.replies, rep)
	return nil
}

func (r *responderFake) AnswerEmbedMessage(_ *adapter.Embed) (string, string, error) {
	return "c1", "m1", nil
}

func (r *responderFake) AnswerEmbedMessageWithButtons(_ *adapter.Embed, _ []adapter.ActionRow) (string, string, error) {
	return "c1", "m1", nil
}

func (r *responderFake) AnswerTextMessageWithButtons(_ string, _ []adapter.ActionRow) (string, string, error) {
	return "c1", "m1", nil
}

func (r *responderFake) EditResponseText(_ string) error { return nil }

func (r *responderFake) ReplaceMessage(rep adapter.Reply) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = append(r.actions, "replace")
	r.replies = append(r.replies, rep)
	return nil
}

func (r *responderFake) OpenModal(_ adapter.Modal) error { return nil }

func (r *responderFake) ResolveDeferred() error { return nil }

func (r *responderFake) lastReply() adapter.Reply {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.replies) == 0 {
		return adapter.Reply{}
	}
	return r.replies[len(r.replies)-1]
}
