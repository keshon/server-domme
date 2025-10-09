package core

import (
	"github.com/bwmarrin/discordgo"
)

func WithAccessControl() Middleware {
	return func(cmd Command) Command {
		return &wrappedCommand{
			Command: cmd,
			wrap: func(ctx interface{}) error {
				var (
					session *discordgo.Session
					member  *discordgo.Member
					event   interface{}
					guildID string
				)

				switch v := ctx.(type) {

				// Slash Command
				case *SlashInteractionContext:
					session, member, event, guildID = v.Session, v.Event.Member, v.Event, v.Event.GuildID

				// Component Interaction (button, menu, etc.)
				case *ComponentInteractionContext:
					session, member, event, guildID = v.Session, v.Event.Member, v.Event, v.Event.GuildID

				// Message Context Menu Command
				case *MessageApplicationCommandContext:
					session, member, event, guildID = v.Session, v.Event.Member, v.Event, v.Event.GuildID

				// Regular message command
				case *MessageContext:
					session, guildID = v.Session, v.Event.GuildID
					if v.Event.Member != nil {
						member = v.Event.Member
					}
				default:
					return nil
				}

				if cmd.RequireAdmin() {
					if !IsAdministrator(session, guildID, member) {
						sendAccessDenied(session, event, "You must be an admin to use this command, darling.")
						return nil
					}
				}

				return cmd.Run(ctx)
			},
		}
	}
}

func sendAccessDenied(session *discordgo.Session, event interface{}, msg string) {
	switch e := event.(type) {
	case *discordgo.InteractionCreate:
		RespondEphemeral(session, e, msg)
	case *discordgo.MessageCreate:
		_, _ = session.ChannelMessageSend(e.ChannelID, msg)
	}
}
