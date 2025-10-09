package core

import (
	"github.com/bwmarrin/discordgo"
)

// WithAccessControl wraps a command to enforce admin-only access if required.
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

				// 	// Skip admin check for normal messages entirely
				// if _, ok := ctx.(*MessageContext); ok {
				// 	return cmd.Run(ctx)
				// }

				// Determine the context type and extract relevant info
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
					session, guildID, event = v.Session, v.Event.GuildID, v.Event
					member = v.Event.Member // can be nil in DMs
				default:
					// Unknown context type, skip
					return nil
				}

				// Check if this command requires admin privileges
				if cmd.RequireAdmin() {
					// If member info or guildID is missing, we cannot check admin status
					if guildID == "" || member == nil {
						sendAccessDenied(ctx, session, event, "Cannot determine your admin status in this context.")
						return nil
					}

					// Check if the user is an administrator
					if !IsAdministrator(session, guildID, member) {
						sendAccessDenied(ctx, session, event, "You must be an admin to use this command, darling.")
						return nil
					}
				}

				// Run the actual command
				return cmd.Run(ctx)
			},
		}
	}
}

// sendAccessDenied sends an appropriate access denied message depending on the context
func sendAccessDenied(ctx interface{}, session *discordgo.Session, event interface{}, msg string) {
	switch e := ctx.(type) {

	case *SlashInteractionContext:
		RespondEphemeral(session, e.Event, msg)

	case *ComponentInteractionContext:
		RespondEphemeral(session, e.Event, msg)

	case *MessageApplicationCommandContext:
		RespondEphemeral(session, e.Event, msg)
	}
}
