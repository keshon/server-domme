package discord

import (
	"context"
	"fmt"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/keshon/command"

	"github.com/keshon/server-domme/internal/discord/audit"
	"github.com/keshon/server-domme/internal/discord/session"
	"github.com/keshon/server-domme/internal/discord/slashsync"
	"github.com/keshon/server-domme/internal/discord/watchdog"
)

// RunSession opens one Discord session and blocks until ctx is cancelled or
// the session is judged unhealthy (transient gateway reconnects do not exit
// this function).
func (b *Bot) RunSession(ctx context.Context) error {
	disconnected := make(chan struct{})
	notifyUnhealthy := b.makeSessionUnhealthyNotifier(disconnected)

	// Built before the session opens so nothing is missed between connecting
	// and wiring. The syncer and recorder need the client, which does not exist
	// yet, so they are filled in once it does.
	var (
		syncer   *slashsync.Syncer
		recorder *audit.Recorder
	)

	tracker := watchdog.NewTracker()

	session, err := session.New(session.Options{
		Token: b.cfg.DiscordToken,
		Log:   b.log,
		Listeners: []bot.EventListener{
			bot.NewListenerFunc(func(_ *events.Raw) { tracker.MarkWSNow() }),
			bot.NewListenerFunc(func(e *events.Ready) {
				tracker.MarkReadyNow()
				b.onReady(e, syncer)
			}),
			bot.NewListenerFunc(func(e *events.GuildJoin) {
				b.onGuildJoin(e, syncer)
			}),
			bot.NewListenerFunc(func(e *events.ApplicationCommandInteractionCreate) {
				b.onApplicationCommand(e, syncer, recorder)
			}),
			bot.NewListenerFunc(func(e *events.ComponentInteractionCreate) {
				b.onComponentInteraction(e, recorder)
			}),
			bot.NewListenerFunc(func(e *events.ModalSubmitInteractionCreate) {
				b.onModalSubmit(e, recorder)
			}),
			bot.NewListenerFunc(func(e *events.MessageReactionAdd) {
				b.onMessageReactionAdd(e)
			}),
			bot.NewListenerFunc(func(e *events.MessageCreate) {
				b.onMessageCreate(e)
			}),
		},
	})
	if err != nil {
		return fmt.Errorf("discord: failed to create session: %w", err)
	}

	client := session.Client()
	syncer = slashsync.NewSyncer(client, command.DefaultRegistry, b.log)
	recorder = audit.NewRecorder(client, b.storage, b.log)

	b.setConn(&conn{client: client})
	defer b.clearConn()

	sessionCtx, cancelSession := context.WithCancel(ctx)
	b.setSessionContext(sessionCtx)
	defer func() {
		cancelSession()
		b.setSessionContext(context.Background())
	}()

	openCtx, cancelOpen := context.WithTimeout(ctx, 30*time.Second)
	defer cancelOpen()
	if err := session.Open(openCtx); err != nil {
		return fmt.Errorf("discord: failed to open Discord session: %w", err)
	}
	defer func() {
		b.log.Info().Msg("discord_session_close")
		// Bounded and abandonable: the usual reason a session is being closed
		// early is that something in it has stopped answering.
		closeWithin("session_close", sessionCloseTimeout, b.log, func() {
			closeCtx, cancelClose := context.WithTimeout(context.Background(), gatewayCloseBudget)
			defer cancelClose()
			session.Close(closeCtx)
		})
	}()

	b.startHealthWatcher(sessionCtx, session, tracker, notifyUnhealthy)

	select {
	case <-ctx.Done():
		b.log.Info().Msg("shutdown_signal_received")
		closeWithin("drain_commands", commandsDrainTimeout, b.log, b.drainCommands)
		return nil
	case <-disconnected:
		return fmt.Errorf("%w: websocket disconnected", ErrSessionUnhealthy)
	}
}

// healthWatchTick is how often the silence watcher checks the gateway, and so
// also how late a repeated signal can land after its timeout.
const healthWatchTick = 10 * time.Second

// startHealthWatcher watches for a gateway that has stopped talking.
//
// One watcher, not two. The discordgo era needed a second to notice a session
// whose lock would never come free and an API probe to catch what the first
// missed. disgo records heartbeats as events, so there is no lock to wedge
// and nothing to time out reading.
func (b *Bot) startHealthWatcher(
	ctx context.Context,
	session *session.Session,
	tracker *watchdog.Tracker,
	notifyUnhealthy func(),
) {
	if mode := b.cfg.DiscordUnhealthyMode; mode != "ignore" &&
		!graceCanEscalate(b.cfg.DiscordUnhealthyGrace, b.cfg.DiscordUnhealthyWindow, b.cfg.WSSilenceTimeout, healthWatchTick) {
		b.log.Warn().
			Int("grace", b.cfg.DiscordUnhealthyGrace).
			Dur("window", b.cfg.DiscordUnhealthyWindow).
			Dur("timeout", b.cfg.WSSilenceTimeout).
			Msg("unhealthy_grace_never_escalates")
	}

	go watchdog.NewWSSilence(
		tracker,
		b.cfg.WSSilenceTimeout,
		session.Latency,
		func(meta watchdog.WSSilenceMeta) {
			b.log.Warn().
				Dur("since_last_ws", meta.SinceLastWS).
				Dur("since_last_heartbeat_ack", meta.SinceLastHeartbeatAck).
				Dur("heartbeat_latency", meta.HeartbeatLatency).
				Dur("timeout", meta.Timeout).
				Msg("gateway_silent")
			notifyUnhealthy()
		},
		watchdog.WSSilenceOptions{
			SettleDelay: 15 * time.Second,
			Tick:        healthWatchTick,
			LastHeartbeatAck: func() (time.Time, bool) {
				return session.LastHeartbeatAck(), true
			},
		},
	).Run(ctx)
}
