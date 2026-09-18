package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/keshon/server-domme/internal/ai"
	chatsvc "github.com/keshon/server-domme/internal/chat"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord"
	"github.com/keshon/server-domme/internal/mind"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// buildChatService assembles the conversational persona, or returns nil and
// the reason why.
//
// Nil is a supported outcome rather than a failure, and every caller treats it
// as one. The persona is optional, it depends on third-party services that may
// be unreachable at boot, and a bot that refuses to start because a free relay
// is down would be trading a working server-management bot for a missing chat
// feature.
//
// The reason is returned rather than only logged because the person who needs
// it is an administrator in Discord, not whoever can read the container logs.
// An earlier version said only "see CHAT_ENABLED", which sent an operator who
// had already set it looking in the wrong place — the actual fault was a
// character file that never reached the deployment.
func buildChatService(
	ctx context.Context,
	cfg *config.Config,
	store *storage.Storage,
	bot *discord.Bot,
	log zerolog.Logger,
) (*chatsvc.Service, string) {
	if !cfg.ChatEnabled {
		return nil, ""
	}

	names := mind.CleanNames(cfg.ChatNames)
	if len(names) == 0 {
		log.Error().Msg("chat_name_missing")
		return nil, "CHAT_NAME is empty, so she has no name to answer to"
	}

	character, err := mind.LoadCharacter(names[0], cfg.ChatCharacterPath)
	if err != nil {
		log.Error().Err(err).
			Str("path", cfg.ChatCharacterPath).
			Msg("chat_character_load_failed")
		return nil, fmt.Sprintf(
			"the character file at `%s` could not be read. In Docker it is read "+
				"from the mounted volume, so it has to exist in the deployment's "+
				"own `data/` directory, not only in the repository.",
			cfg.ChatCharacterPath)
	}

	pool, err := ai.Build(ctx, log, ai.Options{
		UsePollinations: cfg.ChatUsePollinations,
		UseG4F:          cfg.ChatUseG4F,
		G4FPicks:        cfg.ChatG4FPicks,
		G4FAPIKey:       cfg.ChatG4FAPIKey,
		CustomBaseURL:   cfg.ChatBaseURL,
		CustomModel:     cfg.ChatModel,
		CustomAPIKey:    cfg.ChatAPIKey,
		Extra:           cfg.ChatBackends,
		Timeout:         cfg.ChatRequestTimeout,
	})
	if err != nil {
		log.Error().Err(err).Msg("chat_backend_build_failed")
		// Two different mistakes with one symptom. Build does not probe
		// anything, so an empty pool means nothing was configured at all —
		// telling that operator their backends are unreachable sends them to
		// check a network that is working fine.
		if errors.Is(err, ai.ErrNoBackend) {
			return nil, "no backend is configured, so there is nothing to speak " +
				"through. Set `CHAT_BACKENDS` to an OpenAI-compatible endpoint, " +
				"or turn a hosted relay back on with `CHAT_USE_G4F`. Note those " +
				"are separate: switching the relay off does not enable the " +
				"self-hosted one."
		}
		return nil, "no chat backend could be reached at startup. The relays are " +
			"free public services and go down; check the host can reach them, " +
			"then restart."
	}

	// An unknown zone is a misconfiguration, not a reason to refuse to start:
	// she falls back to UTC and says so, which costs the clock its accuracy
	// and nothing else.
	location := time.UTC
	if cfg.ChatTimezone != "" {
		loaded, err := time.LoadLocation(cfg.ChatTimezone)
		if err != nil {
			log.Warn().Err(err).Str("timezone", cfg.ChatTimezone).Msg("chat_timezone_invalid")
		} else {
			location = loaded
		}
	}

	attention := mind.DefaultAttention()
	attention.MentionChance = cfg.ChatMentionChance
	attention.NamedChance = cfg.ChatNamedChance
	attention.AboutChance = cfg.ChatAboutChance
	attention.ReplyChance = cfg.ChatReplyChance
	attention.FollowUpChance = cfg.ChatFollowUpChance

	log.Info().
		Str("character", character.Name).
		Int("examples", len(character.Examples)).
		Msg("chat_character_loaded")

	// bot.Session rather than a session value: RunSession builds a fresh one
	// on every reconnect, and the retry loop outlives any single session.
	return chatsvc.New(chatsvc.Deps{
		Character:      character,
		Provider:       pool,
		Storage:        store,
		Session:        bot.Session,
		Log:            log,
		Names:          names,
		Attention:      attention,
		Location:       location,
		RequestTimeout: cfg.ChatRequestTimeout,
		InnerVoice:     cfg.ChatInnerVoice,
		CasualSlips:    cfg.ChatCasualSlips,
	}), ""
}
