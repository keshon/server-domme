package discord

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"

	"github.com/keshon/command"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

// onMessageCreate feeds message observers and dispatches @mention commands.
func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	msgCtx := &cmdadapter.MessageContext{
		Session: s, Event: m, Storage: b.storage, Config: b.cfg, AppLog: b.log,
	}

	// Observers see every message rather than only the ones addressing the
	// bot, and they run inline rather than through execguard: an observer does
	// in-memory work and at most one storage write, so putting ordinary
	// channel traffic through the command slots would spend all sixteen of
	// them on messages that are not commands.
	//
	// A command that observed the message is then skipped below, or a mention
	// would be handled twice — once as an observation and again as a command.
	rest := make([]command.Command, 0, len(command.DefaultRegistry.GetAll()))
	for _, c := range command.DefaultRegistry.GetAll() {
		if observer, ok := command.Root(c).(cmdadapter.MessageObserverAdapter); ok {
			if observer.ObserveMessage(msgCtx) {
				continue
			}
		}
		rest = append(rest, c)
	}

	mentioned := false
	for _, u := range m.Mentions {
		if u.ID == s.State.User.ID {
			mentioned = true
			break
		}
	}
	if !mentioned {
		return
	}

	b.runWithCommandContext(commandRunOptions{
		onBusy: func(err error) {
			b.log.Warn().Str("kind", "message").Err(err).Msg("command_slot_busy")
		},
	}, func(cmdCtx context.Context) error {
		inv := &command.Invocation{Data: msgCtx}
		for _, c := range rest {
			if err := c.Run(cmdCtx, inv); err != nil {
				if cmdCtx.Err() == context.DeadlineExceeded {
					b.log.Warn().Str("kind", "message").Err(err).Msg("command_timeout")
					_ = reply.MessageEmbed(s, m.ChannelID, &discordgo.MessageEmbed{
						Description: "Timed out running command.",
					})
					continue
				}
				b.log.Error().Str("kind", "message").Err(err).Msg("command_run_error")
				_ = reply.MessageEmbed(s, m.ChannelID, &discordgo.MessageEmbed{
					Description: fmt.Sprintf("Error: %v", err),
				})
			}
		}
		return nil
	})
}

// onMessageReactionAdd handles reaction events for commands that use reactions.
func (b *Bot) onMessageReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	b.mu.RLock()
	logger := b.cmdLogger
	b.mu.RUnlock()

	b.runWithCommandContext(commandRunOptions{
		onBusy: func(err error) {
			b.log.Warn().Str("kind", "reaction").Err(err).Msg("command_slot_busy")
		},
	}, func(cmdCtx context.Context) error {
		inv := &command.Invocation{Data: &cmdadapter.MessageReactionContext{
			Session: s, Event: r, Storage: b.storage, Config: b.cfg, Logger: logger,
			AppLog: b.log,
		}}
		for _, c := range command.DefaultRegistry.GetAll() {
			if _, ok := command.Root(c).(cmdadapter.ReactionProvider); !ok {
				continue
			}
			if err := c.Run(cmdCtx, inv); err != nil {
				if cmdCtx.Err() == context.DeadlineExceeded {
					b.log.Warn().Str("kind", "reaction").Err(err).Msg("command_timeout")
					continue
				}
				b.log.Error().Str("kind", "reaction").Err(err).Msg("command_run_error")
				_ = reply.MessageEmbed(s, r.ChannelID, &discordgo.MessageEmbed{
					Description: fmt.Sprintf("Error: %v", err),
				})
			}
		}
		return nil
	})
}
