package core

import (
	"log"
)

// WithCommandLogger wraps a command to log its execution
func WithCommandLogger() Middleware {
	return func(cmd Command) Command {
		return &wrappedCommand{
			Command: cmd,
			wrap: func(ctx interface{}) error {
				// Run the actual command first
				err := cmd.Run(ctx)

				// Then try to log its execution
				switch v := ctx.(type) {

				//  Slash Command
				case *SlashInteractionContext:
					member := v.Event.Member
					user := member.User
					guildID := v.Event.GuildID
					channelID := v.Event.ChannelID
					if e := LogCommand(v.Session, v.Storage, guildID, channelID, user.ID, user.Username, cmd.Name()); e != nil {
						log.Printf("[WARN] Failed to log command /%s: %v", cmd.Name(), e)
					}

				// Component Interaction (button, menu, etc.)
				case *ComponentInteractionContext:
					member := v.Event.Member
					user := member.User
					guildID := v.Event.GuildID
					channelID := v.Event.ChannelID
					if e := LogCommand(v.Session, v.Storage, guildID, channelID, user.ID, user.Username, cmd.Name()); e != nil {
						log.Printf("[WARN] Failed to log component command /%s: %v", cmd.Name(), e)
					}

				// Message Context Menu Command
				case *MessageApplicationCommandContext:
					member := v.Event.Member
					user := member.User
					guildID := v.Event.GuildID
					channelID := v.Event.ChannelID
					if e := LogCommand(v.Session, v.Storage, guildID, channelID, user.ID, user.Username, cmd.Name()); e != nil {
						log.Printf("[WARN] Failed to log message context /%s: %v", cmd.Name(), e)
					}

				// Regular message command
				case *MessageContext:
					user := v.Event.Author
					guildID := v.Event.GuildID
					channelID := v.Event.ChannelID
					if v.Storage != nil {
						if e := LogCommand(v.Session, v.Storage, guildID, channelID, user.ID, user.Username, cmd.Name()); e != nil {
							log.Printf("[WARN] Failed to log message command /%s: %v", cmd.Name(), e)
						}
					}

				// Reaction command
				case *MessageReactionContext:
					user := v.Event.UserID
					guildID := v.Event.GuildID
					channelID := v.Event.ChannelID
					if v.Storage != nil {
						if e := LogCommand(v.Session, v.Storage, guildID, channelID, user, user, cmd.Name()); e != nil {
							log.Printf("[WARN] Failed to log reaction command /%s: %v", cmd.Name(), e)
						}
					}
				}

				return err
			},
		}
	}
}
