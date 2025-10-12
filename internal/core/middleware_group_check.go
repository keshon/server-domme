package core

import (
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

// WithGroupAccessCheck wraps a command to enforce group access
func WithGroupAccessCheck() Middleware {
	return func(cmd Command) Command {
		return &wrappedCommand{
			Command: cmd,
			wrap: func(ctx interface{}) error {
				var (
					guildID string
					storage *storage.Storage
					respond func(string)
				)

				switch v := ctx.(type) {

				// Slash Command
				case *SlashInteractionContext:
					guildID, storage = v.Event.GuildID, v.Storage
					respond = func(msg string) { RespondEmbedEphemeral(v.Session, v.Event, &discordgo.MessageEmbed{Description: msg}) }

				// Component Interaction (button, menu, etc.)
				case *ComponentInteractionContext:
					guildID, storage = v.Event.GuildID, v.Storage
					respond = func(msg string) { RespondEmbedEphemeral(v.Session, v.Event, &discordgo.MessageEmbed{Description: msg}) }

					if disabledGroup(cmd, guildID, storage, respond) {
						return nil
					}
					if ch, ok := cmd.(ComponentInteractionHandler); ok {
						return ch.Component(v)
					}
					return nil

				// Message Context Menu Command
				case *MessageApplicationCommandContext:
					guildID, storage = v.Event.GuildID, v.Storage
					respond = func(msg string) { RespondEmbedEphemeral(v.Session, v.Event, &discordgo.MessageEmbed{Description: msg}) }

				// Regular message command
				case *MessageContext:
					guildID, storage = v.Event.GuildID, v.Storage
					respond = func(_ string) {}

				// Reaction command
				case *MessageReactionContext:
					guildID, storage = v.Event.GuildID, v.Storage
					respond = func(_ string) {}

				default:
					return nil
				}

				if disabledGroup(cmd, guildID, storage, respond) {
					return nil
				}
				return cmd.Run(ctx)
			},
		}
	}
}

func disabledGroup(cmd Command, guildID string, storage *storage.Storage, respond func(string)) bool {
	if cmd.Group() == "" {
		return false
	}
	disabled, err := storage.IsGroupDisabled(guildID, cmd.Group())
	if err != nil {
		return false
	}
	if disabled {
		respond("This command is disabled on this server.\nUse `/commands status` to check which commands are disabled.")
		return true
	}
	return false
}
