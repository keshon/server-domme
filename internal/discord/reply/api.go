package reply

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
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

// SendChannelReply posts content in reply to a message.
func (a *API) SendChannelReply(channelID, replyToID, content string) error {
	if a.client == nil {
		return fmt.Errorf("reply: no Discord session")
	}
	cid, err := parseID(channelID)
	if err != nil {
		return err
	}
	mid, err := parseID(replyToID)
	if err != nil {
		return err
	}
	_, err = a.client.Rest.CreateMessage(cid, discord.MessageCreate{
		Content:          content,
		MessageReference: &discord.MessageReference{MessageID: &mid, ChannelID: &cid},
	})
	return err
}

// ClearChannelComponents strips the buttons off a message the bot posted.
func (a *API) ClearChannelComponents(channelID, messageID string) error {
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
	empty := []discord.LayoutComponent{}
	_, err = a.client.Rest.UpdateMessage(cid, mid, discord.MessageUpdate{Components: &empty})
	return err
}

// RoleNames resolves a guild's roles to id-indexed names.
func (a *API) RoleNames(guildID string) (map[string]string, error) {
	if a.client == nil {
		return nil, fmt.Errorf("reply: no Discord session")
	}
	gid, err := parseID(guildID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for role := range a.client.Caches.Roles(gid) {
		out[role.ID.String()] = role.Name
	}
	if len(out) > 0 {
		return out, nil
	}
	roles, err := a.client.Rest.GetRoles(gid)
	if err != nil {
		return nil, fmt.Errorf("reply: listing roles: %w", err)
	}
	for _, role := range roles {
		out[role.ID.String()] = role.Name
	}
	return out, nil
}

func (a *API) SendChannelEmbed(channelID string, embed *adapter.Embed) error {
	cid, err := a.channelID(channelID)
	if err != nil {
		return err
	}
	_, err = a.client.Rest.CreateMessage(cid, discord.MessageCreate{Embeds: Embeds(embed)})
	return err
}

// SendChannelEmbedFile posts an embed with a file attached.
func (a *API) SendChannelEmbedFile(channelID string, embed *adapter.Embed, file io.Reader, fileName string) error {
	if a.client == nil {
		return fmt.Errorf("reply: no Discord session")
	}
	cid, err := parseID(channelID)
	if err != nil {
		return err
	}
	_, err = a.client.Rest.CreateMessage(cid, discord.MessageCreate{
		Embeds: Embeds(embed),
		Files:  []*discord.File{discord.NewFile(fileName, "", file)},
	})
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
			ID:   att.ID.String(),
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

// ChannelMessages lists a channel's history newest-first.
func (a *API) ChannelMessages(channelID, beforeID string, limit int) ([]adapter.ListedMessage, error) {
	if a.client == nil {
		return nil, fmt.Errorf("reply: no Discord session")
	}
	cid, err := parseID(channelID)
	if err != nil {
		return nil, err
	}
	var before snowflake.ID
	if beforeID != "" {
		before, err = parseID(beforeID)
		if err != nil {
			return nil, err
		}
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	msgs, err := a.client.Rest.GetMessages(cid, 0, before, 0, limit)
	if err != nil {
		return nil, fmt.Errorf("reply: listing messages: %w", err)
	}
	out := make([]adapter.ListedMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, adapter.ListedMessage{
			ID:        m.ID.String(),
			Timestamp: m.CreatedAt,
		})
	}
	return out, nil
}

// DeleteMessage deletes one message.
func (a *API) DeleteMessage(channelID, messageID string) error {
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
	return a.client.Rest.DeleteMessage(cid, mid)
}

// BulkDeleteMessages deletes a batch in one request. Discord caps the batch
// at 100 and refuses messages older than two weeks; the caller enforces both
// and sends older ones through DeleteMessage instead.
func (a *API) BulkDeleteMessages(channelID string, messageIDs []string) error {
	if a.client == nil {
		return fmt.Errorf("reply: no Discord session")
	}
	cid, err := a.channelID(channelID)
	if err != nil {
		return err
	}
	ids := make([]snowflake.ID, 0, len(messageIDs))
	for _, id := range messageIDs {
		mid, err := parseID(id)
		if err != nil {
			return err
		}
		ids = append(ids, mid)
	}
	return a.client.Rest.BulkDeleteMessages(cid, ids)
}

// AddMemberRole assigns a role to a guild member.
func (a *API) AddMemberRole(guildID, userID, roleID string) error {
	if a.client == nil {
		return fmt.Errorf("reply: no Discord session")
	}
	gid, err := parseID(guildID)
	if err != nil {
		return err
	}
	uid, err := parseID(userID)
	if err != nil {
		return err
	}
	rid, err := parseID(roleID)
	if err != nil {
		return err
	}
	return a.client.Rest.AddMemberRole(gid, uid, rid)
}

// RemoveMemberRole takes a role off a guild member.
func (a *API) RemoveMemberRole(guildID, userID, roleID string) error {
	if a.client == nil {
		return fmt.Errorf("reply: no Discord session")
	}
	gid, err := parseID(guildID)
	if err != nil {
		return err
	}
	uid, err := parseID(userID)
	if err != nil {
		return err
	}
	rid, err := parseID(roleID)
	if err != nil {
		return err
	}
	return a.client.Rest.RemoveMemberRole(gid, uid, rid)
}

// RoleName resolves a role id to its name, for settings that echo what was
// configured.
func (a *API) RoleName(guildID, roleID string) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("reply: no Discord session")
	}
	gid, err := parseID(guildID)
	if err != nil {
		return "", err
	}
	rid, err := parseID(roleID)
	if err != nil {
		return "", err
	}
	if role, ok := a.client.Caches.Role(gid, rid); ok {
		return role.Name, nil
	}
	role, err := a.client.Rest.GetRole(gid, rid)
	if err != nil {
		return "", fmt.Errorf("reply: fetching role: %w", err)
	}
	return role.Name, nil
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

// Member fetches one guild member: what /welcome checks before posting.
func (a *API) Member(guildID, userID string) (*adapter.Member, error) {
	if a.client == nil {
		return nil, fmt.Errorf("reply: no Discord session")
	}
	gid, err := parseID(guildID)
	if err != nil {
		return nil, err
	}
	uid, err := parseID(userID)
	if err != nil {
		return nil, err
	}
	m, err := a.member(gid, uid)
	if err != nil {
		return nil, err
	}
	out := &adapter.Member{
		UserID:     m.User.ID.String(),
		Username:   m.User.Username,
		GlobalName: derefString(m.User.GlobalName),
		Nick:       derefString(m.Nick),
		Bot:        m.User.Bot,
	}
	for _, rid := range m.RoleIDs {
		out.Roles = append(out.Roles, rid.String())
	}
	return out, nil
}

// GuildChannels lists the channels and open threads a template may name.
func (a *API) GuildChannels(guildID string) ([]adapter.Channel, error) {
	if a.client == nil {
		return nil, fmt.Errorf("reply: no Discord session")
	}
	gid, err := parseID(guildID)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var out []adapter.Channel
	for ch := range a.client.Caches.Channels() {
		if ch.GuildID() != gid {
			continue
		}
		id := ch.ID().String()
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, adapter.Channel{ID: id, Name: ch.Name()})
	}
	if channels, err := a.client.Rest.GetGuildChannels(gid); err == nil {
		for _, ch := range channels {
			id := ch.ID().String()
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, adapter.Channel{ID: id, Name: ch.Name()})
		}
	}
	if active, err := a.client.Rest.GetActiveGuildThreads(gid); err == nil {
		for _, t := range active.Threads {
			id := t.ID().String()
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, adapter.Channel{ID: id, Name: t.Name()})
		}
	}
	return out, nil
}

// GuildArchivedThreads lists archived threads under the guild's channels,
// the most recent hundred of each. A channel that cannot be read is skipped.
func (a *API) GuildArchivedThreads(guildID string) ([]adapter.Channel, error) {
	if a.client == nil {
		return nil, fmt.Errorf("reply: no Discord session")
	}
	gid, err := parseID(guildID)
	if err != nil {
		return nil, err
	}
	channels, err := a.client.Rest.GetGuildChannels(gid)
	if err != nil {
		return nil, fmt.Errorf("reply: listing channels: %w", err)
	}
	var parents []snowflake.ID
	for _, ch := range channels {
		switch ch.Type() {
		case discord.ChannelTypeGuildText, discord.ChannelTypeGuildNews,
			discord.ChannelTypeGuildForum, discord.ChannelTypeGuildMedia:
			parents = append(parents, ch.ID())
		}
	}
	// A few at a time: one after another, a server with a few dozen
	// channels keeps the administrator waiting on "thinking".
	found := make([][]adapter.Channel, len(parents))
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i, id := range parents {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			list, err := a.client.Rest.GetPublicArchivedThreads(id, time.Now().UTC(), 100)
			if err != nil || list == nil {
				return
			}
			for _, t := range list.Threads {
				found[i] = append(found[i], adapter.Channel{ID: t.ID().String(), Name: t.Name()})
			}
		}()
	}
	wg.Wait()
	var out []adapter.Channel
	for _, f := range found {
		out = append(out, f...)
	}
	return out, nil
}

// PostMessage posts content with at most one file and explicit mentions, and
// reports the message id.
func (a *API) PostMessage(channelID string, msg adapter.OutgoingMessage) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("reply: no Discord session")
	}
	cid, err := parseID(channelID)
	if err != nil {
		return "", err
	}
	post := discord.MessageCreate{
		Content:         msg.Content,
		AllowedMentions: allowedMentions(msg),
	}
	if msg.File != nil {
		post.Files = []*discord.File{discord.NewFile(msg.FileName, "", msg.File)}
	}
	sent, err := a.client.Rest.CreateMessage(cid, post)
	if err != nil {
		return "", err
	}
	return sent.ID.String(), nil
}

func allowedMentions(msg adapter.OutgoingMessage) *discord.AllowedMentions {
	m := &discord.AllowedMentions{}
	if msg.MentionUser != "" {
		if uid, err := snowflake.Parse(msg.MentionUser); err == nil {
			m.Users = append(m.Users, uid)
		}
	}
	if msg.AllowEveryone {
		m.Parse = append(m.Parse, discord.AllowedMentionTypeEveryone)
	}
	seen := make(map[string]bool)
	for _, id := range msg.AllowRoles {
		if seen[id] {
			continue
		}
		seen[id] = true
		if rid, err := snowflake.Parse(id); err == nil {
			m.Roles = append(m.Roles, rid)
		}
	}
	return m
}

// CanPostIn reports why the bot could not post in a channel, or "" when it
// can.
func (a *API) CanPostIn(channelID, guildID string) string {
	if a.client == nil {
		return "no Discord session"
	}
	cid, err := parseID(channelID)
	if err != nil {
		return fmt.Sprintf("<#%s> no longer exists", channelID)
	}
	ch, ok := a.client.Caches.Channel(cid)
	if !ok {
		fetched, err := a.client.Rest.GetChannel(cid)
		if err != nil || fetched == nil {
			return fmt.Sprintf("<#%s> no longer exists", channelID)
		}
		gch, ok := fetched.(discord.GuildChannel)
		if !ok {
			return fmt.Sprintf("<#%s> is not in this server", channelID)
		}
		ch = gch
	}
	if gid, err := parseID(guildID); err != nil || ch.GuildID() != gid {
		return fmt.Sprintf("<#%s> is not in this server", channelID)
	}
	perms, err := a.botPermissions(channelID)
	if err != nil {
		// Permissions could not be worked out from the cache; let Discord
		// be the judge rather than refusing on a guess.
		return ""
	}
	need := discord.PermissionViewChannel | discord.PermissionSendMessages
	say := "View Channel and Send Messages"
	switch ch.Type() {
	case discord.ChannelTypeGuildNewsThread, discord.ChannelTypeGuildPublicThread,
		discord.ChannelTypeGuildPrivateThread:
		// A thread takes its own permission, which Send Messages does not
		// carry: a welcome pointed at one would pass this check and be
		// refused by Discord.
		need = discord.PermissionViewChannel | discord.PermissionSendMessagesInThreads
		say = "View Channel and Send Messages in Threads"
	}
	if perms&need != need {
		return fmt.Sprintf("I cannot post in <#%s> — give me %s there", channelID, say)
	}
	return ""
}

// CanMentionEveryone reports why @everyone/@here would not ping here, or ""
// when they would.
func (a *API) CanMentionEveryone(channelID string) string {
	perms, err := a.botPermissions(channelID)
	if err != nil {
		return ""
	}
	if perms&discord.PermissionMentionEveryone == 0 {
		return fmt.Sprintf("the text pings @everyone or @here but I do not have Mention Everyone in <#%s>", channelID)
	}
	return ""
}
