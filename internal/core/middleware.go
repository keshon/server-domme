package core

import (
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type Middleware func(Command) Command

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
					// message commands - ignore respond for now due to spamming bug
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
		respond("This command is disabled on this server. Use `/commands-status` to check which commands are disabled.")
		return true
	}
	return false
}

func WithGuildOnly() Middleware {
	return func(cmd Command) Command {
		return &wrappedCommand{
			Command: cmd,
			wrap: func(ctx interface{}) error {
				if v, ok := ctx.(*SlashInteractionContext); ok && v.Event.GuildID == "" {
					return nil
				}
				if v, ok := ctx.(*MessageContext); ok && v.Event.GuildID == "" {
					return nil
				}
				return cmd.Run(ctx)
			},
		}
	}
}

type wrappedCommand struct {
	Command
	wrap func(ctx interface{}) error
}

func (w *wrappedCommand) Run(ctx interface{}) error {
	if w.wrap != nil {
		return w.wrap(ctx)
	}
	return w.Command.Run(ctx)
}

func (w *wrappedCommand) Component(ctx *ComponentInteractionContext) error {
	if w.wrap != nil {
		return w.wrap(ctx)
	}
	if ch, ok := w.Command.(ComponentInteractionHandler); ok {
		return ch.Component(ctx)
	}
	return nil
}

func (w *wrappedCommand) SlashDefinition() *discordgo.ApplicationCommand {
	if sp, ok := w.Command.(SlashProvider); ok {
		return sp.SlashDefinition()
	}
	return nil
}

func (w *wrappedCommand) ContextDefinition() *discordgo.ApplicationCommand {
	if sp, ok := w.Command.(ContextMenuProvider); ok {
		return sp.ContextDefinition()
	}
	return nil
}

func ApplyMiddlewares(cmd Command, mws ...Middleware) Command {
	for _, mw := range mws {
		cmd = mw(cmd)
	}
	return cmd
}
