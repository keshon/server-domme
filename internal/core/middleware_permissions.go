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
				var s *discordgo.Session
				var m *discordgo.Member
				var guildID, channelID string

				switch v := ctx.(type) {
				case *SlashInteractionContext:
					s, m, guildID, channelID = v.Session, v.Event.Member, v.Event.GuildID, v.Event.ChannelID
				case *ComponentInteractionContext:
					s, m, guildID, channelID = v.Session, v.Event.Member, v.Event.GuildID, v.Event.ChannelID
				case *MessageApplicationCommandContext:
					s, m, guildID, channelID = v.Session, v.Event.Member, v.Event.GuildID, v.Event.ChannelID
				case *MessageContext:
					s, m, guildID, channelID = v.Session, v.Event.Member, v.Event.GuildID, v.Event.ChannelID
				default:
					return cmd.Run(ctx) // если контекст неизвестен — просто выполняем
				}

				if guildID == "" || m == nil {
					return cmd.Run(ctx)
				}

				// получаем реальные права пользователя
				memberPerms, err := s.UserChannelPermissions(m.User.ID, channelID)
				if err != nil {
					return fmt.Errorf("failed to get user permissions: %w", err)
				}

				// если у пользователя есть администратор — всё можно
				if memberPerms&discordgo.PermissionAdministrator != 0 {
					return cmd.Run(ctx)
				}

				required := cmd.Permissions()
				if len(required) == 0 {
					return cmd.Run(ctx)
				}

				// проверяем, хватает ли всех нужных прав
				for _, p := range required {
					if memberPerms&p == 0 {
						name := PermissionNames[p]
						if name == "" {
							name = fmt.Sprintf("0x%x", p)
						}

						switch v := ctx.(type) {
						case *SlashInteractionContext:
							RespondEphemeral(s, v.Event, fmt.Sprintf(
								"You lack required permission `%s` to execute this command.", name))
						case *ComponentInteractionContext:
							RespondEphemeral(s, v.Event, fmt.Sprintf(
								"You lack required permission `%s` to execute this action.", name))
						case *MessageApplicationCommandContext:
							RespondEphemeral(s, v.Event, fmt.Sprintf(
								"You lack required permission `%s` to execute this action.", name))
						case *MessageContext:
							_, _ = s.ChannelMessageSend(channelID, fmt.Sprintf(
								"You lack required permission `%s` to execute this command.", name))
						}

						return nil // не выполняем команду
					}
				}

				// всё ок — запускаем команду
				return cmd.Run(ctx)
			},
		}
	}
}

var BotPermissionNames = PermissionNames

func WithBotPermissionCheck() Middleware {
	return func(cmd Command) Command {
		return &wrappedCommand{
			Command: cmd,
			wrap: func(ctx interface{}) error {
				var s *discordgo.Session
				var guildID, channelID string

				switch v := ctx.(type) {
				case *SlashInteractionContext:
					s, guildID, channelID = v.Session, v.Event.GuildID, v.Event.ChannelID
				case *ComponentInteractionContext:
					s, guildID, channelID = v.Session, v.Event.GuildID, v.Event.ChannelID
				case *MessageApplicationCommandContext:
					s, guildID, channelID = v.Session, v.Event.GuildID, v.Event.ChannelID
				case *MessageContext:
					s, guildID, channelID = v.Session, v.Event.GuildID, v.Event.ChannelID
				default:
					return cmd.Run(ctx)
				}

				if guildID == "" {
					return cmd.Run(ctx)
				}

				required := cmd.BotPermissions()
				if len(required) == 0 {
					return cmd.Run(ctx)
				}

				botUser := s.State.User
				if botUser == nil {
					botUser, _ = s.User("@me")
				}

				if botUser == nil {
					return cmd.Run(ctx)
				}

				botPerms, err := s.UserChannelPermissions(botUser.ID, channelID)
				if err != nil {
					return fmt.Errorf("failed to get bot permissions: %w", err)
				}

				for _, p := range required {
					if botPerms&p == 0 {
						name := BotPermissionNames[p]
						if name == "" {
							name = fmt.Sprintf("0x%x", p)
						}

						// Inform the user
						switch v := ctx.(type) {
						case *SlashInteractionContext:
							RespondEphemeral(s, v.Event, fmt.Sprintf(
								"I need the `%s` permission in this channel to run this command.",
								name))
						case *MessageContext:
							_, _ = s.ChannelMessageSend(channelID, fmt.Sprintf(
								"I need the `%s` permission in this channel to run this command.",
								name))
						}

						return nil
					}
				}

				return cmd.Run(ctx)
			},
		}
	}
}
