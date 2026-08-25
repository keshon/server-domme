package discord

import (
	"github.com/bwmarrin/discordgo"
)

// onReady fires on every successful connect/reconnect.
func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	botInfo, err := s.User("@me")
	if err != nil {
		b.log.Warn().Err(err).Msg("bot_user_fetch_failed")
		return
	}

	for _, g := range r.Guilds {
		if b.isGuildBlacklisted(g.ID) {
			b.log.Info().Str("guild_id", g.ID).Msg("guild_blacklisted_leaving")
			if err := s.GuildLeave(g.ID); err != nil {
				b.log.Error().Str("guild_id", g.ID).Err(err).Msg("guild_leave_failed")
			}
			continue
		}
		if b.cfg.InitSlashCommands {
			if err := b.cmdSyncer.SyncGuildCommands(g.ID); err != nil {
				b.log.Error().Str("guild_id", g.ID).Err(err).Msg("commands_sync_failed")
			}
		}
	}

	// Background services are owned by main, not by a gateway event: onReady
	// fires again on every reconnect, and anything started here would either be
	// duplicated or outlive the shutdown signal. Publishing readiness instead
	// keeps the "wait for a live session" need without that lifecycle.
	b.readyOnce.Do(func() { close(b.ready) })

	b.log.Info().Str("username", botInfo.Username).Msg("discord_ready")
}

// onGuildCreate fires when the bot joins a new guild.
func (b *Bot) onGuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	b.log.Info().Str("guild_id", g.ID).Str("guild_name", g.Name).Msg("guild_added")
	if b.isGuildBlacklisted(g.ID) {
		b.log.Info().Str("guild_id", g.ID).Msg("guild_blacklisted_leaving")
		if err := s.GuildLeave(g.ID); err != nil {
			b.log.Error().Str("guild_id", g.ID).Err(err).Msg("guild_leave_failed")
		}
		return
	}
	if b.cfg.InitSlashCommands {
		if err := b.cmdSyncer.SyncGuildCommands(g.ID); err != nil {
			b.log.Error().Str("guild_id", g.ID).Err(err).Msg("commands_sync_failed")
		}
	}
}
