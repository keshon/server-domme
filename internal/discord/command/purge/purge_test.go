package purge

import (
	"io"
	"testing"
	"time"

	"github.com/keshon/server-domme/internal/discord/adapter"
)

// channel is a fake connection holding one page of a channel's history: it
// answers a listing with them, and records bulk and single deletes
// separately so tests can tell which path a message took.
type channel struct {
	messages []adapter.ListedMessage
	deleted  []string
	bulked   [][]string
}

func (c *channel) MemberPermissions(_, _ string) (int64, error) { return 0, nil }
func (c *channel) CheckBotPermissions(_ string) bool            { return true }
func (c *channel) SendChannelMessage(_, _ string) error         { return nil }
func (c *channel) SendChannelReply(_, _, _ string) error        { return nil }
func (c *channel) ClearChannelComponents(_, _ string) error     { return nil }
func (c *channel) SendChannelEmbed(_ string, _ *adapter.Embed) error {
	return nil
}
func (c *channel) SendChannelEmbedFile(_ string, _ *adapter.Embed, _ io.Reader, _ string) error {
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

func (c *channel) BulkDeleteMessages(_ string, messageIDs []string) error {
	c.bulked = append(c.bulked, messageIDs)
	return nil
}

// "Older than a day" deletes what is older than a day and nothing newer. The
// scheduler had the window inverted and deleted the newest messages; the
// command had it reversed and deleted nothing.
func TestDeleteOlderThanDeletesOnlyTheOld(t *testing.T) {
	now := time.Now()
	c := &channel{messages: []adapter.ListedMessage{
		{ID: "new", Timestamp: now.Add(-time.Hour)},
		{ID: "old1", Timestamp: now.Add(-48 * time.Hour)},
		{ID: "old2", Timestamp: now.Add(-72 * time.Hour)},
	}}
	DeleteOlderThan(c, "c", 24*time.Hour, nil)
	if len(c.bulked) != 1 || len(c.bulked[0]) != 2 {
		t.Errorf("bulked %v, want one batch with the two messages older than a day", c.bulked)
	}
	if len(c.deleted) != 0 {
		t.Errorf("deleted %v, want nothing on the single path", c.deleted)
	}
}

// Young messages in the window go out as one bulk batch, not one request
// each: per-message deletes are what kept large purges ricocheting off 429s.
func TestDeleteMessagesBatchesYoungMessages(t *testing.T) {
	now := time.Now()
	c := &channel{messages: []adapter.ListedMessage{
		{ID: "m1", Timestamp: now.Add(-time.Hour)},
		{ID: "m2", Timestamp: now.Add(-2 * time.Hour)},
		{ID: "m3", Timestamp: now.Add(-3 * time.Hour)},
	}}
	DeleteMessages(c, "c", nil, nil, nil)
	if len(c.bulked) != 1 || len(c.bulked[0]) != 3 {
		t.Errorf("bulked %v, want one batch of three", c.bulked)
	}
	if len(c.deleted) != 0 {
		t.Errorf("deleted %v, want nothing on the single path", c.deleted)
	}
}

// A lone young message cannot go bulk (Discord needs 2-100 per call), so it
// falls back to a single delete rather than failing the batch.
func TestDeleteMessagesSingleDeletesALoneMessage(t *testing.T) {
	now := time.Now()
	c := &channel{messages: []adapter.ListedMessage{
		{ID: "solo", Timestamp: now.Add(-time.Hour)},
	}}
	DeleteMessages(c, "c", nil, nil, nil)
	if len(c.deleted) != 1 || c.deleted[0] != "solo" {
		t.Errorf("deleted %v, want the lone message", c.deleted)
	}
	if len(c.bulked) != 0 {
		t.Errorf("bulked %v, want no bulk call for one message", c.bulked)
	}
}

// Messages older than two weeks would fail a whole batch, so they go out one
// at a time while the young ones still batch.
func TestDeleteMessagesSinglesOutOldMessages(t *testing.T) {
	now := time.Now()
	c := &channel{messages: []adapter.ListedMessage{
		{ID: "young1", Timestamp: now.Add(-time.Hour)},
		{ID: "ancient", Timestamp: now.Add(-30 * 24 * time.Hour)},
		{ID: "young2", Timestamp: now.Add(-2 * time.Hour)},
	}}
	DeleteMessages(c, "c", nil, nil, nil)
	if len(c.deleted) != 1 || c.deleted[0] != "ancient" {
		t.Errorf("deleted %v, want only the ancient message", c.deleted)
	}
	if len(c.bulked) != 1 || len(c.bulked[0]) != 2 {
		t.Errorf("bulked %v, want one batch with both young messages", c.bulked)
	}
}
