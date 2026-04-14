package discord

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/commandkit"

	"server-domme/internal/command"
)

// handleRefreshCommands processes a SystemEventRefreshCommands event.
func (b *Bot) handleRefreshCommands(evt SystemEvent) {
	b.mu.RLock()
	dg := b.dg
	mgr := b.cmdManager
	b.mu.RUnlock()

	if dg == nil || mgr == nil {
		return
	}

	appID, err := appID(dg)
	if err != nil {
		log.Printf("[ERR][%s] Failed to resolve app ID: %v", evt.GuildID, err)
		return
	}

	if b.isGuildBlacklisted(evt.GuildID) {
		removeAllCommands(dg, appID, evt.GuildID)
		return
	}

	switch {
	case strings.HasPrefix(evt.Target, "group:"):
		b.refreshGroup(dg, appID, evt.GuildID, strings.TrimPrefix(evt.Target, "group:"))
	case evt.Target == "" || strings.ToLower(evt.Target) == "all":
		_ = mgr.RegisterCommands(evt.GuildID)
	default:
		refreshSingle(dg, appID, evt.GuildID, evt.Target)
	}
}

func removeAllCommands(dg *discordgo.Session, appID, guildID string) {
	log.Printf("[BLACKLIST][%s] Removing all commands", guildID)
	existing, _ := dg.ApplicationCommands(appID, guildID)
	for _, c := range existing {
		if err := dg.ApplicationCommandDelete(appID, guildID, c.ID); err != nil {
			log.Printf("[ERR][%s] Failed to delete %s: %v", guildID, c.Name, err)
		} else {
			log.Printf("[DONE][%s] Deleted %s", guildID, c.Name)
		}
	}
}

func (b *Bot) refreshGroup(dg *discordgo.Session, appID, guildID, group string) {
	disabledGroups, _ := b.storage.GetDisabledGroups(guildID)
	disabled := make(map[string]bool, len(disabledGroups))
	for _, g := range disabledGroups {
		disabled[g] = true
	}

	existing, _ := dg.ApplicationCommands(appID, guildID)
	existingByName := make(map[string]*discordgo.ApplicationCommand, len(existing))
	for _, c := range existing {
		existingByName[c.Name] = c
	}

	for _, c := range commandkit.DefaultRegistry.GetAll() {
		meta, ok := commandkit.Root(c).(command.DiscordMeta)
		if !ok || meta.Group() != group {
			continue
		}
		rc, registered := existingByName[c.Name()]
		if disabled[group] && registered {
			log.Printf("[INFO][%s] Removing disabled command: %s", guildID, c.Name())
			_ = dg.ApplicationCommandDelete(appID, guildID, rc.ID)
		} else if !disabled[group] && !registered {
			if def := commandDefinition(c); def != nil {
				log.Printf("[INFO][%s] Registering enabled command: %s", guildID, c.Name())
				_, _ = dg.ApplicationCommandCreate(appID, guildID, def)
			}
		}
	}
}

func refreshSingle(dg *discordgo.Session, appID, guildID, name string) {
	for _, c := range commandkit.DefaultRegistry.GetAll() {
		if strings.EqualFold(c.Name(), name) {
			if def := commandDefinition(c); def != nil {
				_, _ = dg.ApplicationCommandCreate(appID, guildID, def)
			}
			return
		}
	}
	log.Printf("[WARN][%s] No command found for refresh target: %s", guildID, name)
}

// commandDefinition extracts the ApplicationCommand definition from a registered command,
// walking through middleware wrappers via commandkit.Root.
func commandDefinition(c commandkit.Command) *discordgo.ApplicationCommand {
	root := commandkit.Root(c)
	if slash, ok := root.(command.SlashProvider); ok {
		if def := slash.SlashDefinition(); def != nil {
			if def.Type == 0 {
				def.Type = discordgo.ChatApplicationCommand
			}
			return def
		}
	}
	if menu, ok := root.(command.ContextMenuProvider); ok {
		if def := menu.ContextDefinition(); def != nil {
			if def.Type == 0 {
				def.Type = discordgo.MessageApplicationCommand
			}
			return def
		}
	}
	return nil
}

func appID(dg *discordgo.Session) (string, error) {
	if id := dg.State.User.ID; id != "" {
		return id, nil
	}
	u, err := dg.User("@me")
	if err != nil {
		return "", fmt.Errorf("failed to fetch bot user: %w", err)
	}
	return u.ID, nil
}

