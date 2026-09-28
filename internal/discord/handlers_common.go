package discord

import (
	"context"
	"errors"
	"fmt"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/queue"
)

type commandRunOptions struct {
	onBusy    func(error)
	onTimeout func(error)
	onError   func(error)
}

func (b *Bot) runWithCommandContext(opts commandRunOptions, fn func(cmdCtx context.Context) error) {
	cmdCtx, cancel := b.commandContext()
	defer cancel()

	// Waiting for a slot is bounded even though running is not, because the
	// two are bounded by different things: an interaction that has not been
	// acknowledged within three seconds of being created cannot be answered
	// at all, so a command that has not started by then is better refused
	// than started.
	slotCtx, cancelSlot := context.WithTimeout(cmdCtx, slotWaitBudget)
	err := b.acquireCommandSlot(slotCtx)
	cancelSlot()
	if err != nil {
		if opts.onBusy != nil {
			opts.onBusy(err)
		}
		return
	}
	defer b.releaseCommandSlot()

	if err := fn(cmdCtx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			if opts.onTimeout != nil {
				opts.onTimeout(err)
			}
			return
		}
		if opts.onError != nil {
			opts.onError(err)
		}
	}
}

// runGuardedInteraction runs an interaction under the bot's command guard.
// kind is the dispatch kind ("slash", "component", "modal", "menu"); name is
// the resolved command name.
//
// It runs on a command worker, not on the gateway read goroutine; see
// dispatchInteraction.
func (b *Bot) runGuardedInteraction(
	r adapter.Responder,
	kind string,
	name string,
	fn func(cmdCtx context.Context) error,
) {
	refuse := func(msg string) {
		if r == nil {
			return
		}
		_ = r.Respond(adapter.Reply{Embed: &adapter.Embed{Description: msg}, Ephemeral: true})
	}

	// A command that answered some other way leaves the deferred placeholder
	// with nothing to replace it, so the placeholder is cleared here rather
	// than each command being asked to remember. A normally answered
	// interaction makes this a no-op.
	defer func() {
		if r == nil {
			return
		}
		if err := r.ResolveDeferred(); err != nil {
			b.log.Warn().Str("kind", kind).Str("command", name).Err(err).
				Msg("deferred_placeholder_unresolved")
		}
	}()

	b.runWithCommandContext(commandRunOptions{
		onBusy: func(err error) {
			b.log.Warn().Str("kind", kind).Str("command", name).Err(err).Msg("command_slot_busy")
			refuse("Bot is busy right now. Please try again in a moment.")
		},
		onTimeout: func(err error) {
			b.log.Warn().Str("kind", kind).Str("command", name).Err(err).Msg("command_timeout")
			refuse("Timed out running command.")
		},
		onError: func(err error) {
			b.log.Error().Str("kind", kind).Str("command", name).Err(err).Msg("command_run_error")
			refuse(fmt.Sprintf("Error running command: %v", err))
		},
	}, fn)
}

// dispatchInteraction hands a command body to its guild's lane and returns,
// so the gateway read goroutine goes back to reading the socket.
//
// The lane is the guild, because two of its commands overlapping have no
// meaningful interleaving. Direct messages have no guild, so they lane by
// channel.
func (b *Bot) dispatchInteraction(
	who adapter.Invoker,
	r adapter.Responder,
	kind string,
	name string,
	fn func(cmdCtx context.Context) error,
) {
	run := func() { b.runGuardedInteraction(r, kind, name, fn) }
	if b.commands == nil {
		// No queue means no bot: only a Bot built outside NewBot gets here,
		// and running inline is a better answer than dropping the command.
		run()
		return
	}
	err := b.commands.Submit(commandLane(who), run)
	if err == nil {
		return
	}

	text := "Bot is shutting down."
	if errors.Is(err, queue.ErrLaneFull) {
		text = "Too many commands are waiting on this server. Try again in a moment."
		b.log.Warn().Str("kind", kind).Str("command", name).Msg("command_refused_lane_full")
	} else {
		b.log.Warn().Str("kind", kind).Str("command", name).Err(err).Msg("command_refused_shutting_down")
	}
	if r != nil {
		_ = r.Respond(adapter.Reply{Embed: &adapter.Embed{Description: text}, Ephemeral: true})
	}
}

func commandLane(who adapter.Invoker) string {
	if who.GuildID != "" {
		return who.GuildID
	}
	return "dm:" + who.ChannelID
}

// matchesComponentID reports whether a component or modal customID belongs to
// a command. CustomIDs follow the convention "commandName",
// "commandName:...", or "commandName_...".
func matchesComponentID(customID, commandName string) bool {
	if customID == commandName {
		return true
	}
	if len(customID) > len(commandName) {
		sep := customID[len(commandName)]
		return (sep == ':' || sep == '_') && customID[:len(commandName)] == commandName
	}
	return false
}
