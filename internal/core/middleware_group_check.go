package core

import (
	"server-domme/internal/storage"
)

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
				case *SlashInteractionContext:
					guildID, storage = v.Event.GuildID, v.Storage
					respond = func(msg string) { RespondEphemeral(v.Session, v.Event, msg) }

				case *MessageContext:
					guildID, storage = v.Event.GuildID, v.Storage
					respond = func(_ string) {}

				case *ComponentInteractionContext:
					guildID, storage = v.Event.GuildID, v.Storage
					respond = func(msg string) { RespondEphemeral(v.Session, v.Event, msg) }

					if disabledGroup(cmd, guildID, storage, respond) {
						return nil
					}
					if ch, ok := cmd.(ComponentInteractionHandler); ok {
						return ch.Component(v)
					}
					return nil

				case *MessageApplicationCommandContext:
					guildID, storage = v.Event.GuildID, v.Storage
					respond = func(msg string) { RespondEphemeral(v.Session, v.Event, msg) }

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
		respond("This command is disabled on this server. Use `/cmd-status` to check which commands are disabled.")
		return true
	}
	return false
}
