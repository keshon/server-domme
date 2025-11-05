package middleware

import (
	"log"
	"server-domme/internal/bot"
	"server-domme/internal/command"

	"github.com/bwmarrin/discordgo"
)

// WithCommandLogger wraps a command to log its execution
func WithCommandLogger() command.Middleware {
	return func(cmd command.Command) command.Command {
		return &command.WrappedCommand{
			Command: cmd,
			Wrap: func(ctx interface{}) error {
				err := cmd.Run(ctx)

				switch v := ctx.(type) {

				// Slash Command
				case *command.SlashInteractionContext:
					s := v.Session
					e := v.Event
					guildID := e.GuildID
					channelID := e.ChannelID

					user := resolveUser(s, e)
					if e := bot.LogCommand(s, v.Storage, guildID, channelID, user.ID, user.Username, cmd.Name()); e != nil {
						log.Printf("[WARN] Failed to log command /%s: %v", cmd.Name(), e)
					}

				// Component Interaction
				case *command.ComponentInteractionContext:
					s := v.Session
					e := v.Event
					guildID := e.GuildID
					channelID := e.ChannelID

					user := resolveUser(s, e)
					if e := bot.LogCommand(s, v.Storage, guildID, channelID, user.ID, user.Username, cmd.Name()); e != nil {
						log.Printf("[WARN] Failed to log component /%s: %v", cmd.Name(), e)
					}

				// Context Menu Command
				case *command.MessageApplicationCommandContext:
					s := v.Session
					e := v.Event
					guildID := e.GuildID
					channelID := e.ChannelID

					user := resolveUser(s, e)
					if e := bot.LogCommand(s, v.Storage, guildID, channelID, user.ID, user.Username, cmd.Name()); e != nil {
						log.Printf("[WARN] Failed to log context /%s: %v", cmd.Name(), e)
					}

				// Skip message commands
				case *command.MessageContext:
					return err

				// Reaction Command
				case *command.MessageReactionContext:
					user := v.Event.UserID
					guildID := v.Event.GuildID
					channelID := v.Event.ChannelID
					if v.Storage != nil {
						if e := bot.LogCommand(v.Session, v.Storage, guildID, channelID, user, user, cmd.Name()); e != nil {
							log.Printf("[WARN] Failed to log reaction /%s: %v", cmd.Name(), e)
						}
					}
				}

				return err
			},
		}
	}
}

// resolveUser safely retrieves the user object from an InteractionCreate event
func resolveUser(s *discordgo.Session, e *discordgo.InteractionCreate) *discordgo.User {
	if e.Member != nil && e.Member.User != nil {
		return e.Member.User
	}
	if e.User != nil {
		return e.User
	}

	// As last resort, try fetching from Discord API
	if e.Member != nil && e.Member.User != nil {
		return e.Member.User
	}

	// If we know the user ID but not username — fetch it
	if e.User != nil {
		if u, err := s.User(e.User.ID); err == nil {
			return u
		}
	}
	// Safe fallback
	return &discordgo.User{ID: "unknown", Username: "Unknown"}
}
