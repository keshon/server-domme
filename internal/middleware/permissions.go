package middleware

import (
	"context"
	"fmt"
	"strings"

	"github.com/keshon/command"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"

	"github.com/bwmarrin/discordgo"
)

var PermissionNames = map[int64]string{
	discordgo.PermissionCreateInstantInvite:              "Create Instant Invite",
	discordgo.PermissionKickMembers:                      "Kick Members",
	discordgo.PermissionBanMembers:                       "Ban Members",
	discordgo.PermissionAdministrator:                    "Administrator",
	discordgo.PermissionManageChannels:                   "Manage Channels",
	discordgo.PermissionManageGuild:                      "Manage Server",
	discordgo.PermissionAddReactions:                     "Add Reactions",
	discordgo.PermissionViewAuditLogs:                    "View Audit Logs",
	discordgo.PermissionViewChannel:                      "View Channel",
	discordgo.PermissionSendMessages:                     "Send Messages",
	discordgo.PermissionSendTTSMessages:                  "Send TTS Messages",
	discordgo.PermissionManageMessages:                   "Manage Messages",
	discordgo.PermissionEmbedLinks:                       "Embed Links",
	discordgo.PermissionAttachFiles:                      "Attach Files",
	discordgo.PermissionReadMessageHistory:               "Read Message History",
	discordgo.PermissionMentionEveryone:                  "Mention Everyone",
	discordgo.PermissionUseExternalEmojis:                "Use External Emojis",
	discordgo.PermissionUseApplicationCommands:           "Use Application Commands",
	discordgo.PermissionManageThreads:                    "Manage Threads",
	discordgo.PermissionCreatePublicThreads:              "Create Public Threads",
	discordgo.PermissionCreatePrivateThreads:             "Create Private Threads",
	discordgo.PermissionUseExternalStickers:              "Use External Stickers",
	discordgo.PermissionSendMessagesInThreads:            "Send Messages in Threads",
	discordgo.PermissionSendVoiceMessages:                "Send Voice Messages",
	discordgo.PermissionSendPolls:                        "Send Polls",
	discordgo.PermissionUseExternalApps:                  "Use External Apps",
	discordgo.PermissionVoicePrioritySpeaker:             "Priority Speaker",
	discordgo.PermissionVoiceStreamVideo:                 "Stream Video",
	discordgo.PermissionVoiceConnect:                     "Connect to Voice Channel",
	discordgo.PermissionVoiceSpeak:                       "Speak",
	discordgo.PermissionVoiceMuteMembers:                 "Mute Members",
	discordgo.PermissionVoiceDeafenMembers:               "Deafen Members",
	discordgo.PermissionVoiceMoveMembers:                 "Move Members",
	discordgo.PermissionVoiceUseVAD:                      "Use Voice Activity Detection",
	discordgo.PermissionVoiceRequestToSpeak:              "Request to Speak",
	discordgo.PermissionUseEmbeddedActivities:            "Use Embedded Activities",
	discordgo.PermissionUseSoundboard:                    "Use Soundboard",
	discordgo.PermissionUseExternalSounds:                "Use External Sounds",
	discordgo.PermissionChangeNickname:                   "Change Nickname",
	discordgo.PermissionManageNicknames:                  "Manage Nicknames",
	discordgo.PermissionManageRoles:                      "Manage Roles",
	discordgo.PermissionManageWebhooks:                   "Manage Webhooks",
	discordgo.PermissionManageGuildExpressions:           "Manage Expressions (Emojis, Stickers, Sounds)",
	discordgo.PermissionManageEvents:                     "Manage Events",
	discordgo.PermissionViewCreatorMonetizationAnalytics: "View Creator Monetization Analytics",
	discordgo.PermissionCreateGuildExpressions:           "Create Expressions (Emojis, Stickers, Sounds)",
	discordgo.PermissionCreateEvents:                     "Create Events",
	discordgo.PermissionViewGuildInsights:                "View Guild Insights",
	discordgo.PermissionModerateMembers:                  "Moderate Members",
}

func WithUserPermissionCheck() command.Middleware {
	return func(c command.Command) command.Command {
		return command.Wrap(c, func(ctx context.Context, inv *command.Invocation) error {
			var s *discordgo.Session
			var m *discordgo.Member
			var guildID, channelID string

			switch v := inv.Data.(type) {
			case *cmdadapter.SlashInteractionContext:
				s, m, guildID, channelID = v.Session, v.Event.Member, v.Event.GuildID, v.Event.ChannelID
			case *cmdadapter.ComponentInteractionContext:
				s, m, guildID, channelID = v.Session, v.Event.Member, v.Event.GuildID, v.Event.ChannelID
			case *cmdadapter.MessageApplicationCommandContext:
				s, m, guildID, channelID = v.Session, v.Event.Member, v.Event.GuildID, v.Event.ChannelID
			case *cmdadapter.MessageContext:
				s, m, guildID, channelID = v.Session, v.Event.Member, v.Event.GuildID, v.Event.ChannelID
			default:
				return c.Run(ctx, inv)
			}

			if guildID == "" || m == nil {
				return c.Run(ctx, inv)
			}
			if m.User == nil {
				return c.Run(ctx, inv)
			}

			// DEVELOPER_ID runs everything, everywhere.
			//
			// It exists so the person maintaining the bot can exercise admin
			// commands on a live server without being given a role there, or
			// waking an admin in another timezone to grant one. perm.IsAdministrator
			// already honours it; this check is what extends that to the
			// UserPermissions gate every command declares, which is the one
			// that actually refuses them.
			//
			// It is a total bypass of this middleware, so the id is worth
			// treating as a credential: whoever holds it can run /purge on any
			// guild the bot is in. It does not bypass Discord itself — the bot
			// still needs its own permissions for anything it goes on to do.
			//
			// This runs before UserChannelPermissions on purpose. That call
			// fails outright when the member or channel is not in state, and a
			// developer locked out by a lookup error is exactly the situation
			// this exists to avoid.
			if config.IsDeveloper(cmdadapter.ConfigFromInvocation(inv), m.User.ID) {
				return c.Run(ctx, inv)
			}

			memberPerms, err := s.UserChannelPermissions(m.User.ID, channelID)
			if err != nil {
				return fmt.Errorf("middleware: get user permissions: %w", err)
			}
			if memberPerms&discordgo.PermissionAdministrator != 0 {
				return c.Run(ctx, inv)
			}

			meta, ok := command.Root(c).(cmdadapter.Meta)
			if !ok {
				return c.Run(ctx, inv)
			}
			required := meta.UserPermissions()
			if len(required) == 0 {
				return c.Run(ctx, inv)
			}

			hasAny := false
			for _, p := range required {
				if memberPerms&p != 0 {
					hasAny = true
					break
				}
			}
			if !hasAny {
				var allowed []string
				for _, p := range required {
					name := PermissionNames[p]
					if name == "" {
						name = fmt.Sprintf("0x%x", p)
					}
					allowed = append(allowed, name)
				}
				msg := fmt.Sprintf(
					"You need at least one of the following permissions to run this command:\n`%s`",
					strings.Join(allowed, "`, `"),
				)
				switch v := inv.Data.(type) {
				case *cmdadapter.SlashInteractionContext:
					if v.Responder != nil {
						_ = v.Responder.RespondEmbedEphemeral(s, v.Event, &discordgo.MessageEmbed{Description: msg})
					}
				case *cmdadapter.ComponentInteractionContext:
					if v.Responder != nil {
						_ = v.Responder.RespondEmbedEphemeral(s, v.Event, &discordgo.MessageEmbed{Description: msg})
					}
				case *cmdadapter.MessageApplicationCommandContext:
					if v.Responder != nil {
						_ = v.Responder.RespondEmbedEphemeral(s, v.Event, &discordgo.MessageEmbed{Description: msg})
					}
				case *cmdadapter.MessageContext:
					_, _ = s.ChannelMessageSend(channelID, msg)
				}
				return nil
			}
			return c.Run(ctx, inv)
		})
	}
}
