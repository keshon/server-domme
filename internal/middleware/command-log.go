package middleware

import (
	"context"
	"log"
	"server-domme/internal/bot"
	"server-domme/internal/command"
	"server-domme/pkg/cmd"

	"github.com/bwmarrin/discordgo"
)

// WithCommandLogger wraps a command to log its execution
func WithCommandLogger() cmd.Middleware {
	return func(c cmd.Command) cmd.Command {
		return cmd.Wrap(c, func(ctx context.Context, inv *cmd.Invocation) error {
			err := c.Run(ctx, inv)

			switch v := inv.Data.(type) {
			case *command.SlashInteractionContext:
				e := v.Event
				user := resolveUser(v.Session, e)
				if e := bot.LogCommand(v.Session, v.Storage, e.GuildID, e.ChannelID, user.ID, user.Username, c.Name()); e != nil {
					log.Printf("[WARN] Failed to log command /%s: %v", c.Name(), e)
				}
			case *command.ComponentInteractionContext:
				e := v.Event
				user := resolveUser(v.Session, e)
				if e := bot.LogCommand(v.Session, v.Storage, e.GuildID, e.ChannelID, user.ID, user.Username, c.Name()); e != nil {
					log.Printf("[WARN] Failed to log component /%s: %v", c.Name(), e)
				}
			case *command.MessageApplicationCommandContext:
				e := v.Event
				user := resolveUser(v.Session, e)
				if e := bot.LogCommand(v.Session, v.Storage, e.GuildID, e.ChannelID, user.ID, user.Username, c.Name()); e != nil {
					log.Printf("[WARN] Failed to log context /%s: %v", c.Name(), e)
				}
			case *command.MessageContext:
				// skip message commands
			case *command.MessageReactionContext:
				if v.Storage != nil {
					user := v.Event.UserID
					if e := bot.LogCommand(v.Session, v.Storage, v.Event.GuildID, v.Event.ChannelID, user, user, c.Name()); e != nil {
						log.Printf("[WARN] Failed to log reaction /%s: %v", c.Name(), e)
					}
				}
			}
			return err
		})
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
