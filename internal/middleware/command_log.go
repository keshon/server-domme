package middleware

import (
	"context"
	"log"

	"server-domme/internal/command"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/commandkit"
)

// WithCommandLogger wraps a command to log its execution
func WithCommandLogger() commandkit.Middleware {
	return func(c commandkit.Command) commandkit.Command {
		return commandkit.Wrap(c, func(ctx context.Context, inv *commandkit.Invocation) error {
			err := c.Run(ctx, inv)
			logInvocation(c.Name(), inv)
			return err
		})
	}
}

func logInvocation(cmdName string, inv *commandkit.Invocation) {
	switch v := inv.Data.(type) {
	case *command.SlashInteractionContext:
		logInteraction(cmdName, v.Logger, v.Session, v.Event)
	case *command.ComponentInteractionContext:
		logInteraction(cmdName, v.Logger, v.Session, v.Event)
	case *command.MessageApplicationCommandContext:
		logInteraction(cmdName, v.Logger, v.Session, v.Event)
	case *command.MessageReactionContext:
		if v.Logger != nil {
			logEntry(cmdName, v.Logger, v.Event.GuildID, v.Event.ChannelID, v.Event.UserID, v.Event.UserID)
		}
	case *command.MessageContext:
		// skip message commands
	default:
		return
	}
}

func logInteraction(cmdName string, logger command.Logger, s *discordgo.Session, e *discordgo.InteractionCreate) {
	if logger == nil {
		return
	}
	user := resolveUser(s, e)
	logEntry(cmdName, logger, e.GuildID, e.ChannelID, user.ID, user.Username)
}

func logEntry(cmdName string, logger command.Logger, guildID, channelID, userID, username string) {
	if err := logger.LogCommand(guildID, channelID, userID, username, cmdName); err != nil {
		log.Printf("[WARN] Failed to log command %q: %v", cmdName, err)
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

