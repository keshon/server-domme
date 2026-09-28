package discord

import (
	"context"
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/keshon/command"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/audit"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/discord/slashsync"
)

// interactionInvoker reads the caller off an interaction, once, so nothing
// downstream has to hold the event to ask again.
func interactionInvoker(i discord.Interaction) adapter.Invoker {
	who := adapter.Invoker{
		UserID:   adapter.UnknownUserID,
		Username: adapter.UnknownUsername,
	}
	if gid := i.GuildID(); gid != nil {
		who.GuildID = gid.String()
	}
	if cid := i.Channel().ID(); cid != 0 {
		who.ChannelID = cid.String()
	}

	user := i.User()
	if member := i.Member(); member != nil {
		user = member.User
		// Discord computed these for this channel, overwrites included.
		who.Permissions = int64(member.Permissions)
		who.PermissionsKnown = true
		for _, rid := range member.RoleIDs {
			who.Roles = append(who.Roles, rid.String())
		}
	}
	if user.ID != 0 {
		who.UserID = user.ID.String()
		who.Username = user.Username
		who.DisplayName = user.Username
		if user.GlobalName != nil && *user.GlobalName != "" {
			who.DisplayName = *user.GlobalName
		}
	}
	return who
}

// onReady fires on every successful connect/reconnect.
func (b *Bot) onReady(e *events.Ready, syncer *slashsync.Syncer) {
	for _, g := range e.Guilds {
		guildID := g.ID.String()
		if b.isGuildBlacklisted(guildID) {
			b.log.Info().Str("guild_id", guildID).Msg("guild_blacklisted_leaving")
			if err := e.Client().Rest.LeaveGuild(g.ID); err != nil {
				b.log.Error().Str("guild_id", guildID).Err(err).Msg("guild_leave_failed")
			}
			continue
		}
		if b.cfg.InitSlashCommands {
			if err := syncer.SyncGuildCommands(guildID); err != nil {
				// Whatever was registered before is still registered: the
				// commands live on Discord's side, and a sync that fails
				// changes nothing there. The next connect retries anyway.
				b.log.Error().
					Str("guild_id", guildID).
					Str("impact", "existing commands unaffected; retried on next connect").
					Err(err).
					Msg("commands_sync_failed")
			}
		}
	}
	b.markReady()
	b.log.Info().Str("username", e.User.Username).Msg("discord_ready")
}

// onGuildJoin fires when the bot joins a new guild.
func (b *Bot) onGuildJoin(e *events.GuildJoin, syncer *slashsync.Syncer) {
	guildID := e.Guild.ID.String()
	b.log.Info().Str("guild_id", guildID).Str("guild_name", e.Guild.Name).Msg("guild_added")

	if b.isGuildBlacklisted(guildID) {
		b.log.Info().Str("guild_id", guildID).Msg("guild_blacklisted_leaving")
		if err := e.Client().Rest.LeaveGuild(e.Guild.ID); err != nil {
			b.log.Error().Str("guild_id", guildID).Err(err).Msg("guild_leave_failed")
		}
		return
	}
	if b.cfg.InitSlashCommands {
		if err := syncer.SyncGuildCommands(guildID); err != nil {
			b.log.Error().Str("guild_id", guildID).Err(err).Msg("commands_sync_failed")
		}
	}
}

// onApplicationCommand dispatches slash and message context-menu commands.
func (b *Bot) onApplicationCommand(
	e *events.ApplicationCommandInteractionCreate,
	syncer *slashsync.Syncer,
	recorder *audit.Recorder,
) {
	name := e.Data.CommandName()
	c := command.DefaultRegistry.Get(name)
	if c == nil {
		b.log.Warn().Str("command", name).Msg("command_unknown")
		return
	}

	responder := reply.NewCommandResponder(e)
	api := reply.NewSessionAPI(e.Client())
	who := interactionInvoker(e)

	switch data := e.Data.(type) {
	case discord.SlashCommandInteractionData:
		attachments := make(map[string]adapter.Attachment, len(data.Resolved.Attachments))
		for id, att := range data.Resolved.Attachments {
			attachments[id.String()] = adapter.Attachment{
				ID:   id.String(),
				Name: att.Filename,
				URL:  att.URL,
			}
		}
		inv := &command.Invocation{Data: &adapter.SlashInteractionContext{
			Invoker: who, Responder: responder, API: api,
			Arguments:   reply.SlashArguments(data),
			Attachments: attachments,
			Storage:     b.storage, Config: b.cfg, Audit: recorder, AppLog: b.log,
			Syncer: syncer,
		}}
		b.dispatchInteraction(who, responder, "slash", name, func(cmdCtx context.Context) error {
			return c.Run(cmdCtx, inv)
		})
	case discord.MessageCommandInteractionData:
		inv := &command.Invocation{Data: &adapter.MessageCommandContext{
			Invoker: who, Responder: responder, API: api,
			TargetMessageID: data.TargetID().String(),
			Storage:         b.storage, Config: b.cfg, Audit: recorder, AppLog: b.log,
			Syncer: syncer,
		}}
		b.dispatchInteraction(who, responder, "menu", name, func(cmdCtx context.Context) error {
			// Through middleware, like every other invocation: the group
			// check and the audit trail apply to menus too.
			return c.Run(cmdCtx, inv)
		})
	default:
		b.log.Warn().Str("command", name).Str("kind", fmt.Sprintf("%T", e.Data)).
			Msg("interaction_kind_unhandled")
	}
}

// onComponentInteraction dispatches a click on a message component.
func (b *Bot) onComponentInteraction(e *events.ComponentInteractionCreate, recorder *audit.Recorder) {
	customID := e.Data.CustomID()
	b.log.Debug().Str("custom_id", customID).Msg("component_interaction")

	var matched command.Command
	for _, c := range command.DefaultRegistry.GetAll() {
		if matchesComponentID(customID, c.Name()) {
			matched = c
			break
		}
	}
	if matched == nil {
		b.log.Warn().Str("custom_id", customID).Msg("component_no_handler")
		return
	}

	if _, ok := command.Root(matched).(adapter.ComponentInteractionHandler); !ok {
		b.log.Warn().Str("command", matched.Name()).Msg("component_handler_missing")
		return
	}

	responder := reply.NewComponentResponder(e)
	who := interactionInvoker(e)
	var firstEmbed *adapter.Embed
	if len(e.Message.Embeds) > 0 {
		firstEmbed = reply.FromWire(e.Message.Embeds[0])
	}
	cc := &adapter.ComponentInteractionContext{
		Invoker:        who,
		Responder:      responder,
		API:            reply.NewSessionAPI(e.Client()),
		ComponentID:    customID,
		MessageID:      e.Message.ID.String(),
		MessageContent: e.Message.Content,
		MessageEmbed:   firstEmbed,
		Storage:        b.storage, Config: b.cfg, Audit: recorder, AppLog: b.log,
	}

	b.dispatchInteraction(who, responder, "component", matched.Name(), func(cmdCtx context.Context) error {
		return runComponent(cmdCtx, matched, cc)
	})
}

// runComponent runs a click through the command it belongs to, middleware and
// all, exactly as a slash invocation of that command would run.
func runComponent(ctx context.Context, matched command.Command, cc *adapter.ComponentInteractionContext) error {
	return matched.Run(ctx, &command.Invocation{Data: cc})
}

// onModalSubmit dispatches a modal editor submission.
func (b *Bot) onModalSubmit(e *events.ModalSubmitInteractionCreate, recorder *audit.Recorder) {
	customID := e.Data.CustomID
	b.log.Debug().Str("custom_id", customID).Msg("modal_submit")

	var matched command.Command
	for _, c := range command.DefaultRegistry.GetAll() {
		if matchesComponentID(customID, c.Name()) {
			matched = c
			break
		}
	}
	if matched == nil {
		b.log.Warn().Str("custom_id", customID).Msg("modal_no_handler")
		return
	}

	if _, ok := command.Root(matched).(adapter.ModalSubmitHandler); !ok {
		b.log.Warn().Str("command", matched.Name()).Msg("modal_handler_missing")
		return
	}

	responder := reply.NewModalResponder(e)
	who := interactionInvoker(e)
	values := make(map[string]string, len(e.Data.Components))
	for component := range e.Data.AllComponents() {
		if ic, ok := component.(discord.InteractiveComponent); ok {
			if ti, ok := component.(discord.TextInputComponent); ok {
				values[ic.GetCustomID()] = ti.Value
			}
		}
	}
	mc := &adapter.ModalSubmitContext{
		Invoker:     who,
		Responder:   responder,
		API:         reply.NewSessionAPI(e.Client()),
		ComponentID: customID,
		Values:      values,
		Storage:     b.storage, Config: b.cfg, Audit: recorder, AppLog: b.log,
	}

	b.dispatchInteraction(who, responder, "modal", matched.Name(), func(cmdCtx context.Context) error {
		return matched.Run(cmdCtx, &command.Invocation{Data: mc})
	})
}

// onMessageReactionAdd feeds flag reactions to the commands that ask for
// them. The bot's own reactions are skipped: they are cleanup markers, not
// user requests.
func (b *Bot) onMessageReactionAdd(e *events.MessageReactionAdd) {
	self, ok := e.Client().Caches.SelfUser()
	if ok && e.UserID == self.ID {
		return
	}

	who := adapter.Invoker{
		UserID:   adapter.UnknownUserID,
		Username: adapter.UnknownUsername,
	}
	if e.GuildID != nil {
		who.GuildID = e.GuildID.String()
	}
	who.ChannelID = e.ChannelID.String()
	who.UserID = e.UserID.String()
	if e.Member != nil {
		who.Username = e.Member.User.Username
		who.DisplayName = e.Member.User.Username
		if e.Member.User.GlobalName != nil && *e.Member.User.GlobalName != "" {
			who.DisplayName = *e.Member.User.GlobalName
		}
		for _, rid := range e.Member.RoleIDs {
			who.Roles = append(who.Roles, rid.String())
		}
		// Effective permissions are resolved on demand through the API: the
		// gateway member carries roles, not the channel-resolved bits that
		// interactions deliver precomputed.
	}

	rc := &adapter.ReactionContext{
		Invoker:   who,
		API:       reply.NewSessionAPI(e.Client()),
		MessageID: e.MessageID.String(),
		Emoji:     e.Emoji.Reaction(),
		Storage:   b.storage, Config: b.cfg, AppLog: b.log,
	}

	for _, c := range command.DefaultRegistry.GetAll() {
		root, ok := command.Root(c).(adapter.ReactionHandler)
		if !ok {
			continue
		}
		name := c.Name()
		// No responder: a reaction cannot be answered, only acted on. A busy
		// queue drops it with a log line rather than a reply nobody can see.
		b.dispatchInteraction(who, nil, "reaction", name, func(cmdCtx context.Context) error {
			return root.React(rc)
		})
	}
}
