package discord

import (
	"time"

	"github.com/rs/zerolog"
)

// sessionCloseTimeout is the outer bound: how long teardown may take before
// it is abandoned and the process moves on regardless.
const sessionCloseTimeout = 15 * time.Second

// gatewayCloseBudget is what the library's own Close is given, and it is
// deliberately much shorter than the abandon above: a close that waits out a
// rate-limit window is waiting for requests nobody is going to make, because
// the process is leaving.
const gatewayCloseBudget = 3 * time.Second

// commandsDrainTimeout bounds waiting for the commands already running to
// finish. A command is a REST round trip or two; anything past this is a
// command that is not coming back, and the process is leaving either way.
const commandsDrainTimeout = 5 * time.Second

// closeWithin runs a teardown step, gives up waiting for it after timeout, and
// records how long it actually took.
//
// Do NOT replace this with a bare call. Teardown is the last thing a session
// does, so a step that blocks blocks the restart loop in main with it, and
// the bot a watchdog just correctly declared dead never comes back.
func closeWithin(phase string, timeout time.Duration, log zerolog.Logger, fn func()) {
	started := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		log.Info().Str("phase", phase).Dur("took", time.Since(started)).
			Msg("shutdown_phase")
	case <-timer.C:
		log.Warn().Str("phase", phase).Dur("timeout", timeout).
			Msg("shutdown_phase_abandoned")
	}
}
