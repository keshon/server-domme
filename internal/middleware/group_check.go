package middleware

import (
	"context"

	"github.com/keshon/command"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

// WithGroupAccessCheck wraps a command to enforce group access
func WithGroupAccessCheck() command.Middleware {
	return func(c command.Command) command.Command {
		return command.Wrap(c, func(ctx context.Context, inv *command.Invocation) error {
			var (
				guildID string
				stor    *storage.Storage
				respond func(string)
				event   *discordgo.InteractionCreate
			)

			switch v := inv.Data.(type) {
			case *cmdadapter.SlashInteractionContext:
				guildID, stor, event = v.Event.GuildID, v.Storage, v.Event
				if v.Responder != nil {
					respond = func(msg string) {
						_ = v.Responder.RespondEmbedEphemeral(v.Session, v.Event, &discordgo.MessageEmbed{Description: msg})
					}
				} else {
					respond = func(_ string) {}
				}
			case *cmdadapter.ComponentInteractionContext:
				guildID, stor = v.Event.GuildID, v.Storage
				if v.Responder != nil {
					respond = func(msg string) {
						_ = v.Responder.RespondEmbedEphemeral(v.Session, v.Event, &discordgo.MessageEmbed{Description: msg})
					}
				} else {
					respond = func(_ string) {}
				}
				if disabledGroup(c, guildID, stor, nil, respond) {
					return nil
				}
				if ch, ok := command.Root(c).(cmdadapter.ComponentInteractionHandler); ok {
					return ch.Component(v)
				}
				return nil
			case *cmdadapter.MessageApplicationCommandContext:
				guildID, stor = v.Event.GuildID, v.Storage
				if v.Responder != nil {
					respond = func(msg string) {
						_ = v.Responder.RespondEmbedEphemeral(v.Session, v.Event, &discordgo.MessageEmbed{Description: msg})
					}
				} else {
					respond = func(_ string) {}
				}
			case *cmdadapter.MessageContext:
				guildID, stor = v.Event.GuildID, v.Storage
				respond = func(_ string) {}
			case *cmdadapter.MessageReactionContext:
				guildID, stor = v.Event.GuildID, v.Storage
				respond = func(_ string) {}
			default:
				return c.Run(ctx, inv)
			}

			if disabledGroup(c, guildID, stor, event, respond) {
				return nil
			}
			return c.Run(ctx, inv)
		})
	}
}

func disabledGroup(c command.Command, guildID string, stor *storage.Storage, event *discordgo.InteractionCreate, respond func(string)) bool {
	meta, ok := command.Root(c).(cmdadapter.Meta)
	if !ok || meta.Group() == "" {
		return false
	}

	group := meta.Group()
	if group == "core" && c.Name() == "settings" && event != nil {
		if featureGroup := settingsFeatureGroup(event); featureGroup != "" && featureGroup != "core" {
			group = featureGroup
		}
	}

	disabled, err := stor.IsGroupDisabled(guildID, group)
	if err != nil {
		return false
	}
	if disabled {
		respond("This command is disabled on this server.\nUse `/settings commands status` to check which commands are disabled.")
		return true
	}
	return false
}

func settingsFeatureGroup(event *discordgo.InteractionCreate) string {
	if event == nil || event.Type != discordgo.InteractionApplicationCommand {
		return ""
	}
	data := event.ApplicationCommandData()
	if data.Name != "settings" || len(data.Options) == 0 {
		return ""
	}
	groupOpt := data.Options[0]
	if groupOpt.Type != discordgo.ApplicationCommandOptionSubCommandGroup {
		return ""
	}
	switch groupOpt.Name {
	case "announce", "confess", "discipline", "media", "task", "translate":
		return groupOpt.Name
	case "commands":
		return "core"
	default:
		return ""
	}
}
