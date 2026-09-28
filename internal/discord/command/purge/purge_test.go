package purge

import (
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/discord/adapter"
)

// channel is a fake connection holding one page of a channel's history: it
// answers a listing with them, and records what is deleted.
type channel struct {
	messages []adapter.ListedMessage
	deleted  []string
}

func (c *channel) MemberPermissions(_, _ string) (int64, error) { return 0, nil }
func (c *channel) CheckBotPermissions(_ string) bool            { return true }
func (c *channel) SendChannelMessage(_, _ string) error         { return nil }
func (c *channel) SendChannelReply(_, _, _ string) error        { return nil }
func (c *channel) ClearChannelComponents(_, _ string) error     { return nil }
func (c *channel) SendChannelEmbed(_ string, _ *adapter.Embed) error {
	return nil
}
func (c *channel) SendDirectMessage(_, _ string) error { return nil }
func (c *channel) ChannelMessage(_, _ string) (*adapter.Message, error) {
	return &adapter.Message{}, nil
}
func (c *channel) ForwardMessage(_ string, _ *adapter.Message) error { return nil }
func (c *channel) RemoveReaction(_, _, _, _ string) error            { return nil }
func (c *channel) GuildMembers(_ string) ([]adapter.GuildMember, error) {
	return nil, nil
}
func (c *channel) Member(_, _ string) (*adapter.Member, error) {
	return &adapter.Member{}, nil
}
func (c *channel) GuildChannels(_ string) ([]adapter.Channel, error) {
	return nil, nil
}
func (c *channel) GuildArchivedThreads(_ string) ([]adapter.Channel, error) {
	return nil, nil
}
func (c *channel) PostMessage(_ string, _ adapter.OutgoingMessage) (string, error) {
	return "m1", nil
}
func (c *channel) CanPostIn(_, _ string) string       { return "" }
func (c *channel) CanMentionEveryone(_ string) string { return "" }
func (c *channel) RoleNames(_ string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (c *channel) AddMemberRole(_, _, _ string) error    { return nil }
func (c *channel) RemoveMemberRole(_, _, _ string) error { return nil }
func (c *channel) RoleName(_, _ string) (string, error)  { return "", nil }
func (c *channel) GuildInfo(_ string) (adapter.GuildInfo, error) {
	return adapter.GuildInfo{}, nil
}
func (c *channel) Latency() time.Duration { return 0 }
func (c *channel) EmbedColor() int        { return 0 }
func (c *channel) ChannelMessages(_, _ string, _ int) ([]adapter.ListedMessage, error) {
	return c.messages, nil
}
func (c *channel) DeleteMessage(_, messageID string) error {
	c.deleted = append(c.deleted, messageID)
	return nil
}

// "Older than a day" deletes what is older than a day and nothing newer. The
// scheduler had the window inverted and deleted the newest messages; the
// command had it reversed and deleted nothing.
func TestDeleteOlderThanDeletesOnlyTheOld(t *testing.T) {
	now := time.Now()
	c := &channel{messages: []adapter.ListedMessage{
		{ID: "new", Timestamp: now.Add(-time.Hour)},
		{ID: "old", Timestamp: now.Add(-48 * time.Hour)},
	}}
	DeleteOlderThan(c, "c", 24*time.Hour, nil)
	if len(c.deleted) != 1 || c.deleted[0] != "old" {
		t.Errorf("deleted %v, want only the message older than a day", c.deleted)
	}
}
