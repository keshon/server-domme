package reply

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/keshon/server-domme/internal/discord/adapter"
)

// The connection-level answers. Everything here reads from disgo's cache
// first and falls back to REST only where the cache cannot answer at all,
// which is the same order the discordgo backend uses and for the same reason:
// these run on the command path.

// MemberPermissions resolves a caller's effective permissions in a channel.
//
// discordgo computed this itself from roles and overwrites on every call;
// disgo's cache does the same resolution behind MemberPermissionsInChannel.
// Both answer from cached state, so both are only as current as the cache --
// which is what a permission check on the command path wants, because the
// alternative is two API calls before every refusal.
func (a *API) MemberPermissions(userID, channelID string) (int64, error) {
	if a.client == nil {
		return 0, nil
	}
	uid, err := parseID(userID)
	if err != nil {
		return 0, nil
	}
	cid, err := parseID(channelID)
	if err != nil {
		return 0, nil
	}
	channel, ok := a.client.Caches.Channel(cid)
	if !ok {
		return 0, fmt.Errorf("reply: channel %s not in cache", channelID)
	}
	member, err := a.member(channel.GuildID(), uid)
	if err != nil {
		return 0, err
	}
	return int64(a.client.Caches.MemberPermissionsInChannel(channel, member)), nil
}

// CheckBotPermissions reports whether the bot may manage messages here.
func (a *API) CheckBotPermissions(channelID string) bool {
	perms, err := a.botPermissions(channelID)
	if err != nil {
		return false
	}
	return perms.Has(discord.PermissionManageMessages)
}

func (a *API) botPermissions(channelID string) (discord.Permissions, error) {
	if a.client == nil {
		return 0, fmt.Errorf("reply: no Discord session")
	}
	cid, err := parseID(channelID)
	if err != nil {
		return 0, err
	}
	channel, ok := a.client.Caches.Channel(cid)
	if !ok {
		return 0, fmt.Errorf("reply: channel %s not in cache", channelID)
	}
	selfUser, ok := a.client.Caches.SelfUser()
	if !ok {
		return 0, fmt.Errorf("reply: bot user not known yet")
	}
	self, err := a.member(channel.GuildID(), selfUser.ID)
	if err != nil {
		return 0, err
	}
	return a.client.Caches.MemberPermissionsInChannel(channel, self), nil
}

// member answers from the cache, and asks Discord when the cache has never
// heard of this one.
//
// The cache only knows members it has been told about: GUILD_CREATE carries
// them only for guilds under Discord's large-member threshold, and past that
// the rest are learned from events a music bot has no other reason to
// subscribe to. A permission check that depends on which events happen to be
// enabled is a command that works in one guild and not the next -- so where
// the answer is missing it is fetched rather than turned into a refusal.
func (a *API) member(guildID, userID snowflake.ID) (discord.Member, error) {
	if member, ok := a.client.Caches.Member(guildID, userID); ok {
		return member, nil
	}
	member, err := a.client.Rest.GetMember(guildID, userID)
	if err != nil {
		return discord.Member{}, fmt.Errorf("reply: fetching member %s: %w", userID, err)
	}
	a.client.Caches.AddMember(*member)
	return *member, nil
}

func (a *API) SendChannelMessage(channelID, content string) error {
	cid, err := a.channelID(channelID)
	if err != nil {
		return err
	}
	_, err = a.client.Rest.CreateMessage(cid, discord.MessageCreate{Content: content})
	return err
}

func (a *API) SendChannelEmbed(channelID string, embed *adapter.Embed) error {
	cid, err := a.channelID(channelID)
	if err != nil {
		return err
	}
	_, err = a.client.Rest.CreateMessage(cid, discord.MessageCreate{Embeds: Embeds(embed)})
	return err
}

// SendDirectMessage DMs a user one message. Closed DMs fail here, and the
// caller decides whether that is worth reporting: for ask's notifications it
// is the common case and the outcome is already recorded on the message.
func (a *API) SendDirectMessage(userID, content string) error {
	if a.client == nil {
		return fmt.Errorf("reply: no Discord session")
	}
	uid, err := parseID(userID)
	if err != nil {
		return err
	}
	dm, err := a.client.Rest.CreateDMChannel(uid)
	if err != nil {
		return fmt.Errorf("reply: opening DM channel: %w", err)
	}
	_, err = a.client.Rest.CreateMessage(dm.ID(), discord.MessageCreate{Content: content})
	return err
}

// ChannelMessage fetches one message and renders it neutrally.
func (a *API) ChannelMessage(channelID, messageID string) (*adapter.Message, error) {
	if a.client == nil {
		return nil, fmt.Errorf("reply: no Discord session")
	}
	cid, err := parseID(channelID)
	if err != nil {
		return nil, err
	}
	mid, err := parseID(messageID)
	if err != nil {
		return nil, err
	}
	msg, err := a.client.Rest.GetMessage(cid, mid)
	if err != nil {
		return nil, fmt.Errorf("reply: fetching message: %w", err)
	}
	out := &adapter.Message{ID: msg.ID.String(), Content: msg.Content}
	for _, e := range msg.Embeds {
		e := e
		out.Embeds = append(out.Embeds, FromWire(e))
	}
	for _, att := range msg.Attachments {
		out.Attachments = append(out.Attachments, adapter.Attachment{
			Name: att.Filename,
			URL:  att.URL,
		})
	}
	return out, nil
}

// ForwardMessage reposts a fetched message: content, embeds, attachments.
func (a *API) ForwardMessage(targetChannelID string, msg *adapter.Message) error {
	if a.client == nil {
		return fmt.Errorf("reply: no Discord session")
	}
	if msg == nil {
		return fmt.Errorf("reply: nothing to forward")
	}
	tcid, err := parseID(targetChannelID)
	if err != nil {
		return err
	}
	post := discord.MessageCreate{Content: msg.Content}
	for _, e := range msg.Embeds {
		post.Embeds = append(post.Embeds, Embed(e))
	}
	for _, att := range msg.Attachments {
		data, ferr := fetchAttachment(att.URL)
		if ferr != nil {
			continue
		}
		post.Files = append(post.Files, discord.NewFile(att.Name, "", bytes.NewReader(data)))
	}
	_, err = a.client.Rest.CreateMessage(tcid, post)
	return err
}

// attachmentFetchTimeout bounds one attachment download. The CDN is the only
// host reached here, and a stalled fetch would otherwise hold a command slot
// open for as long as the connection stays half-open.
const attachmentFetchTimeout = 30 * time.Second

func fetchAttachment(url string) ([]byte, error) {
	client := &http.Client{Timeout: attachmentFetchTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reply: attachment fetch: unexpected status %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// RemoveReaction takes one user's reaction off a message.
func (a *API) RemoveReaction(channelID, messageID, emoji, userID string) error {
	if a.client == nil {
		return fmt.Errorf("reply: no Discord session")
	}
	cid, err := parseID(channelID)
	if err != nil {
		return err
	}
	mid, err := parseID(messageID)
	if err != nil {
		return err
	}
	uid, err := parseID(userID)
	if err != nil {
		return err
	}
	return a.client.Rest.RemoveUserReaction(cid, mid, emoji, uid)
}

// GuildMembers lists a guild's members for name resolution.
func (a *API) GuildMembers(guildID string) ([]adapter.GuildMember, error) {
	if a.client == nil {
		return nil, fmt.Errorf("reply: no Discord session")
	}
	gid, err := parseID(guildID)
	if err != nil {
		return nil, err
	}
	var out []adapter.GuildMember
	for member := range a.client.Caches.Members(gid) {
		out = append(out, adapter.GuildMember{
			UserID:     member.User.ID.String(),
			Username:   member.User.Username,
			Nick:       derefString(member.Nick),
			GlobalName: derefString(member.User.GlobalName),
		})
	}
	if len(out) > 0 {
		return out, nil
	}
	members, err := a.client.Rest.GetMembers(gid, 1000, 0)
	if err != nil {
		return nil, fmt.Errorf("reply: listing members: %w", err)
	}
	for _, member := range members {
		out = append(out, adapter.GuildMember{
			UserID:     member.User.ID.String(),
			Username:   member.User.Username,
			Nick:       derefString(member.Nick),
			GlobalName: derefString(member.User.GlobalName),
		})
	}
	return out, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (a *API) PostChannelEmbed(channelID string, embed *adapter.Embed) (string, error) {
	cid, err := a.channelID(channelID)
	if err != nil {
		return "", err
	}
	msg, err := a.client.Rest.CreateMessage(cid, discord.MessageCreate{Embeds: Embeds(embed)})
	if err != nil {
		return "", err
	}
	return msg.ID.String(), nil
}

func (a *API) EditChannelEmbed(channelID, messageID string, embed *adapter.Embed) error {
	cid, err := a.channelID(channelID)
	if err != nil {
		return err
	}
	mid, err := parseID(messageID)
	if err != nil {
		return err
	}
	embeds := []discord.Embed{Embed(embed)}
	_, err = a.client.Rest.UpdateMessage(cid, mid, discord.MessageUpdate{Embeds: &embeds})
	return err
}

// GuildInfo describes a guild. The counts are whatever the cache holds, which
// is what the status command has always reported.
func (a *API) GuildInfo(guildID string) (adapter.GuildInfo, error) {
	if a.client == nil {
		return adapter.GuildInfo{}, fmt.Errorf("reply: no Discord session")
	}
	gid, err := parseID(guildID)
	if err != nil {
		return adapter.GuildInfo{}, err
	}
	guild, ok := a.client.Caches.Guild(gid)
	if !ok {
		fetched, err := a.client.Rest.GetGuild(gid, false)
		if err != nil {
			return adapter.GuildInfo{}, err
		}
		guild = fetched.Guild
	}

	channels := 0
	for ch := range a.client.Caches.Channels() {
		if ch.GuildID() == gid {
			channels++
		}
	}

	return adapter.GuildInfo{
		ID:       guild.ID.String(),
		Name:     guild.Name,
		Members:  a.client.Caches.MembersLen(gid),
		Roles:    a.client.Caches.RolesLen(gid),
		Channels: channels,
	}, nil
}

func (a *API) channelID(channelID string) (snowflake.ID, error) {
	if a.client == nil {
		return 0, fmt.Errorf("reply: no Discord session")
	}
	return parseID(channelID)
}

// parseID turns one of the string ids the neutral layer speaks back into a
// snowflake. The seam uses strings because that is the one spelling both
// libraries agree on -- discordgo has no id type at all -- so every disgo
// entry point converts here.
func parseID(s string) (snowflake.ID, error) {
	if s == "" {
		return 0, fmt.Errorf("reply: empty id")
	}
	id, err := snowflake.Parse(s)
	if err != nil {
		return 0, fmt.Errorf("reply: parsing id %q: %w", s, err)
	}
	return id, nil
}
