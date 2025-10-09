package core

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

// PermissionNames maps permission bit flags to readable names.
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

func WithPermissionCheck() Middleware {
	return func(cmd Command) Command {
		return &wrappedCommand{
			Command: cmd,
			wrap: func(ctx interface{}) error {
				var session *discordgo.Session
				var member *discordgo.Member
				var guildID, channelID string
				var event interface{}

				switch v := ctx.(type) {
				case *SlashInteractionContext:
					session, member, guildID, channelID, event = v.Session, v.Event.Member, v.Event.GuildID, v.Event.ChannelID, v.Event
				case *ComponentInteractionContext:
					session, member, guildID, channelID, event = v.Session, v.Event.Member, v.Event.GuildID, v.Event.ChannelID, v.Event
				case *MessageApplicationCommandContext:
					session, member, guildID, channelID, event = v.Session, v.Event.Member, v.Event.GuildID, v.Event.ChannelID, v.Event
				case *MessageContext:
					session, member, guildID, channelID, event = v.Session, v.Event.Member, v.Event.GuildID, v.Event.ChannelID, v.Event
				default:
					return nil
				}

				perms := cmd.Permissions()
				if len(perms) == 0 {
					return cmd.Run(ctx)
				}

				if guildID == "" || member == nil {
					return nil
				}

				memberPerms, err := session.State.UserChannelPermissions(member.User.ID, channelID)
				if err != nil {
					return nil
				}

				for _, p := range perms {
					if memberPerms&p == 0 {
						name := PermissionNames[p]
						if name == "" {
							name = fmt.Sprintf("%d", p)
						}

						switch e := event.(type) {
						case *discordgo.InteractionCreate:
							RespondEphemeral(session, e, fmt.Sprintf(
								"You lack required permission `%s` to execute this command.", name))
						case *discordgo.MessageCreate:
							_, _ = session.ChannelMessageSend(e.ChannelID, fmt.Sprintf(
								"You lack required permission `%s` to execute this command.", name))
						}
						return nil
					}
				}

				return cmd.Run(ctx)
			},
		}
	}
}
