package middleware

import (
	"context"
	"server-domme/internal/bot"
	"server-domme/internal/command"
	"server-domme/internal/storage"
	"server-domme/pkg/cmd"

	"github.com/bwmarrin/discordgo"
)

// WithGroupAccessCheck wraps a command to enforce group access
func WithGroupAccessCheck() cmd.Middleware {
	return func(c cmd.Command) cmd.Command {
		return cmd.Wrap(c, func(ctx context.Context, inv *cmd.Invocation) error {
			var (
				guildID string
				stor    *storage.Storage
				respond func(string)
			)

			switch v := inv.Data.(type) {
			case *command.SlashInteractionContext:
				guildID, stor = v.Event.GuildID, v.Storage
				respond = func(msg string) {
					bot.RespondEmbedEphemeral(v.Session, v.Event, &discordgo.MessageEmbed{Description: msg})
				}
			case *command.ComponentInteractionContext:
				guildID, stor = v.Event.GuildID, v.Storage
				respond = func(msg string) {
					bot.RespondEmbedEphemeral(v.Session, v.Event, &discordgo.MessageEmbed{Description: msg})
				}
				if disabledGroup(c, guildID, stor, respond) {
					return nil
				}
				if ch, ok := cmd.Root(c).(command.ComponentInteractionHandler); ok {
					return ch.Component(v)
				}
				return nil
			case *command.MessageApplicationCommandContext:
				guildID, stor = v.Event.GuildID, v.Storage
				respond = func(msg string) {
					bot.RespondEmbedEphemeral(v.Session, v.Event, &discordgo.MessageEmbed{Description: msg})
				}
			case *command.MessageContext:
				guildID, stor = v.Event.GuildID, v.Storage
				respond = func(_ string) {}
			case *command.MessageReactionContext:
				guildID, stor = v.Event.GuildID, v.Storage
				respond = func(_ string) {}
			default:
				return c.Run(ctx, inv)
			}

			if disabledGroup(c, guildID, stor, respond) {
				return nil
			}
			return c.Run(ctx, inv)
		})
	}
}

func disabledGroup(c cmd.Command, guildID string, stor *storage.Storage, respond func(string)) bool {
	meta, ok := cmd.Root(c).(command.DiscordMeta)
	if !ok || meta.Group() == "" {
		return false
	}
	disabled, err := stor.IsGroupDisabled(guildID, meta.Group())
	if err != nil {
		return false
	}
	if disabled {
		respond("This command is disabled on this server.\nUse `/commands status` to check which commands are disabled.")
		return true
	}
	return false
}
