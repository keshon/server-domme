package main

import (
	"context"
	"os"

	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/session"
	"github.com/rs/zerolog"
)

// runConnectionCheck connects and reports what it can see, registering
// nothing and sending nothing.
//
// A compile proves nothing about the token, the intents, whether READY
// arrives, or whether REST authenticates — and narrowing the gateway intents
// is exactly the kind of change whose failure mode is a gateway that refuses
// the connection, or a bot that connects and then cannot answer a command.
func runConnectionCheck(ctx context.Context, cfg *config.Config, log zerolog.Logger) {
	res, err := session.Check(ctx, cfg.DiscordToken, log)
	if err != nil {
		log.Error().Err(err).Msg("connection_check_failed")
		os.Exit(1)
	}
	log.Info().
		Str("username", res.Username).
		Int("guilds", res.Guilds).
		Dur("latency", res.Latency).
		Int("existing_commands", res.Commands).
		Str("commands_guild", res.CommandsGuild).
		Msg("connection_check_ok")
}
