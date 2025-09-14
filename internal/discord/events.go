package discord

import (
	"context"
	"log"
	"server-domme/internal/core"
)

func (b *Bot) handleSystemEvents(ctx context.Context) {
	for {
		select {
		case ev := <-core.SystemEvents():
			switch ev.Type {
			case core.SystemEventRefreshCommands:
				log.Printf("[INFO] Refreshing commands for guild %s (target: %s)", ev.GuildID, ev.Target)
				if ev.Target == "all" {
					if err := b.registerCommands(ev.GuildID); err != nil {
						log.Printf("[ERR] Failed to refresh all commands: %v", err)
					}
				} else {
					cmd, ok := core.GetCommand(ev.Target)
					if !ok {
						log.Printf("[ERR] Command not found: %s", ev.Target)
						continue
					}
					def := normalizeDefinition(cmd)
					if def == nil {
						log.Printf("[ERR] No slash/context definition for: %s", ev.Target)
						continue
					}
					_, err := b.dg.ApplicationCommandCreate(b.dg.State.User.ID, ev.GuildID, def)
					if err != nil {
						log.Printf("[ERR] Failed to update command %s: %v", ev.Target, err)
					} else {
						log.Printf("[DONE] Updated command: %s", ev.Target)
					}
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
