package command

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
				var respond func(msg string)

				switch v := ctx.(type) {
				case *SlashContext:
					guildID = v.Event.GuildID
					storage = v.Storage
					respond = func(msg string) {
						respondEphemeral(v.Session, v.Event, msg)
					}
				case *ComponentContext:
					guildID = v.Event.GuildID
					storage = v.Storage
					respond = func(msg string) {
						respondEphemeral(v.Session, v.Event, msg)
					}
				case *ReactionContext:
					guildID = v.Reaction.GuildID
					storage = v.Storage
					respond = func(msg string) {
						// Здесь тебе нужно будет дописать отправку DM или лог в канал
					}
				default:
					return cmd.Run(ctx)
				}

				if cmd.Group() != "" {
					disabled, err := storage.IsGroupDisabled(guildID, cmd.Group())
					if err == nil && disabled {
						respond("This group of commands is disabled on this server.")
						return nil
					}
				}
				return cmd.Run(ctx)
			},
		}
	}
}

func WithGuildOnly(cmd Command) Command {
	return &wrappedCommand{
		Command: cmd,
		wrap: func(ctx interface{}) error {
			switch v := ctx.(type) {
			case *SlashContext:
				if v.Event.GuildID == "" {
					_ = v.Session.InteractionRespond(v.Event.Interaction, &discordgo.InteractionResponse{
						Type: discordgo.InteractionResponseChannelMessageWithSource,
						Data: &discordgo.InteractionResponseData{
							Content: "You must be in a guild to use this command.",
							Flags:   discordgo.MessageFlagsEphemeral,
						},
					})
					return nil
				}
			}
			return cmd.Run(ctx)
		},
	}
}

type wrappedCommand struct {
	Command
	wrap func(ctx interface{}) error
}

func (w *wrappedCommand) Run(ctx interface{}) error {
	return w.wrap(ctx)
}

// Proxy the SlashProvider if the original command implements it
func (w *wrappedCommand) SlashDefinition() *discordgo.ApplicationCommand {
	if slash, ok := w.Command.(SlashProvider); ok {
		return slash.SlashDefinition()
	}
	return nil
}

// Proxy the ContextMenuProvider if the original command implements it
func (w *wrappedCommand) ContextDefinition() *discordgo.ApplicationCommand {
	if menu, ok := w.Command.(ContextMenuProvider); ok {
		return menu.ContextDefinition()
	}
	return nil
}
