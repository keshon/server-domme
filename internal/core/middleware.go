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
				var guildID string
				var storage *storage.Storage

				switch v := ctx.(type) {
				case *SlashContext:
					guildID = v.Event.GuildID
					storage = v.Storage

				case *MessageContext:
					guildID = v.Event.GuildID
					storage = v.Storage

				case *ComponentContext:
					guildID = v.Event.GuildID
					storage = v.Storage
				default:
					return cmd.Run(ctx)
				}

				if cmd.Group() != "" {
					disabled, err := storage.IsGroupDisabled(guildID, cmd.Group())
					if err == nil && disabled {
						return nil
					}
				}

				switch v := ctx.(type) {
				case *MessageContext:
					if mh, ok := cmd.(MessageHandler); ok {
						return mh.Message(v)
					}
				case *SlashContext:
					return cmd.Run(v)
				case *ComponentContext:
					if ch, ok := cmd.(ComponentHandler); ok {
						return ch.Component(v)
					}
				case *ReactionContext, *MessageApplicationContext:
					return cmd.Run(ctx)
				default:
					return cmd.Run(ctx)
				}

				return nil
			},
		}
	}
}

func WithGuildOnly() Middleware {
	return func(cmd Command) Command {
		return &wrappedCommand{
			Command: cmd,
			wrap: func(ctx interface{}) error {
				if v, ok := ctx.(*SlashContext); ok && v.Event.GuildID == "" {
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

func (w *wrappedCommand) Message(ctx *MessageContext) error {
	if w.wrap != nil {
		return w.wrap(ctx)
	}
	if mh, ok := w.Command.(MessageHandler); ok {
		return mh.Message(ctx)
	}
	return nil
}

func (w *wrappedCommand) Component(ctx *ComponentContext) error {
	if w.wrap != nil {
		return w.wrap(ctx)
	}
	if ch, ok := w.Command.(ComponentHandler); ok {
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
