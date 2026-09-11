package main

import (
	"context"

	"github.com/keshon/server-domme/internal/ai"
	chatsvc "github.com/keshon/server-domme/internal/chat"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// buildChatService assembles the conversational persona, or returns nil.
//
// Nil is a supported outcome rather than a failure, and every caller treats it
// as one. The persona is optional, it depends on third-party services that may
// be unreachable at boot, and a bot that refuses to start because a free relay
// is down would be trading a working server-management bot for a missing chat
// feature. Each failure is logged with what it cost.
func buildChatService(
	ctx context.Context,
	cfg *config.Config,
	store *storage.Storage,
	bot *discord.Bot,
	log zerolog.Logger,
) *chatsvc.Service {
	if !cfg.ChatEnabled {
		return nil
	}

	names := mind.CleanNames(cfg.ChatNames)
	if len(names) == 0 {
		log.Error().Msg("chat_name_missing")
		return nil
	}

	character, err := mind.LoadCharacter(names[0], cfg.ChatCharacterPath)
	if err != nil {
		log.Error().Err(err).
			Str("path", cfg.ChatCharacterPath).
			Msg("chat_character_load_failed")
		return nil
	}

	pool, err := ai.Build(ctx, log, ai.Options{
		UsePollinations: cfg.ChatUsePollinations,
		UseG4F:          cfg.ChatUseG4F,
		G4FPicks:        cfg.ChatG4FPicks,
		CustomBaseURL:   cfg.ChatBaseURL,
		CustomModel:     cfg.ChatModel,
		CustomAPIKey:    cfg.ChatAPIKey,
	})
	if err != nil {
		log.Error().Err(err).Msg("chat_backend_build_failed")
		return nil
	}

	attention := mind.DefaultAttention()
	attention.MentionChance = cfg.ChatMentionChance
	attention.NamedChance = cfg.ChatNamedChance
	attention.ReplyChance = cfg.ChatReplyChance
	attention.FollowUpChance = cfg.ChatFollowUpChance

	log.Info().
		Str("character", character.Name).
		Int("examples", len(character.Examples)).
		Msg("chat_character_loaded")

	// bot.Session rather than a session value: RunSession builds a fresh one
	// on every reconnect, and the retry loop outlives any single session.
	return chatsvc.New(chatsvc.Deps{
		Character: character,
		Provider:  pool,
		Storage:   store,
		Session:   bot.Session,
		Log:       log,
		Names:     names,
		Attention: attention,
	})
}
