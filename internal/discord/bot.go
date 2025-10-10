// /internal/discord/bot.go
package discord

import (
	"context"
	"fmt"
	"log"
	"server-domme/internal/config"
	"server-domme/internal/core"
	"server-domme/internal/music/player"
	"server-domme/internal/music/source_resolver"
	"server-domme/internal/storage"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Bot is a Discord bot
type Bot struct {
	dg        *discordgo.Session
	storage   *storage.Storage
	slashCmds map[string][]*discordgo.ApplicationCommand

	mu             sync.RWMutex
	sourceResolver *source_resolver.SourceResolver
	players        map[string]*player.Player
}

// StartBot starts the Discord bot
func StartBot(ctx context.Context, token string, storage *storage.Storage) error {
	b := &Bot{
		storage:   storage,
		slashCmds: make(map[string][]*discordgo.ApplicationCommand),
		players:   make(map[string]*player.Player),
	}
	if err := b.run(ctx, token); err != nil {
		return fmt.Errorf("bot run error: %w", err)
	}
	return nil
}

// run starts the Discord bot
func (b *Bot) run(ctx context.Context, token string) error {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	b.dg = dg

	b.configureIntents()
	dg.AddHandler(b.onReady)
	dg.AddHandler(b.onMessageCreate)
	dg.AddHandler(b.onMessageReactionAdd)
	dg.AddHandler(b.onInteractionCreate)
	dg.AddHandler(b.onGuildCreate)

	if err := dg.Open(); err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}
	defer dg.Close()

	go b.handleSystemEvents(ctx)

	<-ctx.Done()
	log.Println("[INFO] ❎ Shutdown signal received. Cleaning up...")
	return nil
}

// configureIntents configures the Discord intents
func (b *Bot) configureIntents() {
	b.dg.Identify.Intents = discordgo.IntentsAll
}

// onMessageCreate is called when a message is created
func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	mentioned := false
	for _, user := range m.Mentions {
		if user.ID == s.State.User.ID {
			mentioned = true
			break
		}
	}
	if !mentioned {
		return
	}

	for _, cmd := range core.AllCommands() {
		ctx := &core.MessageContext{
			Session: s,
			Event:   m,
			Storage: b.storage,
		}
		err := cmd.Run(ctx)
		if err != nil {
			log.Println("[ERR] Error running command:", err)
			core.MessageEmbed(s, m.ChannelID, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Error running command: %v", err),
			})
		}
	}
}

// onReady is called when the bot is ready
func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	botInfo, err := s.User("@me")
	if err != nil {
		log.Println("[WARN] Error retrieving bot user:", err)
		return
	}

	b.registerMusicCommands()

	cfg := config.New()
	if cfg.InitSlashCommands {
		log.Println("[INFO] Registering slash commands...")
		for _, g := range r.Guilds {
			if err := b.registerCommands(g.ID); err != nil {
				log.Println("[ERR] Error registering slash commands for guild", g.ID, ":", err)
			}
		}
	} else {
		log.Println("[INFO] Registering slash commands skipped")
	}

	log.Println("[INFO] Starting scheduled purge jobs...")
	startScheduledPurgeJobs(b.storage, s)

	if err := updateReadme(); err != nil {
		log.Println("[ERR] Failed to update README:", err)
	}

	log.Printf("[INFO] ✅ Discord bot %v is running.", botInfo.Username)
}

// onGuildCreate is called when a guild is created
func (b *Bot) onGuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	log.Printf("[INFO] Bot added to guild: %s (%s)", g.Guild.ID, g.Guild.Name)

	if err := b.registerCommands(g.Guild.ID); err != nil {
		log.Printf("[ERR] Failed to register commands for new guild %s: %v", g.Guild.ID, err)
	}
}

// onMessageReactionAdd is called when a reaction is added
func (b *Bot) onMessageReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	for _, cmd := range core.AllCommands() {
		if _, ok := cmd.(core.ReactionProvider); ok {
			ctx := &core.MessageReactionContext{
				Session: s,
				Event:   r,
				Storage: b.storage,
			}
			err := cmd.Run(ctx)
			if err != nil {
				log.Println("[ERR] Error running reaction command:", err)
				core.MessageEmbed(s, r.ChannelID, &discordgo.MessageEmbed{
					Description: fmt.Sprintf("Error running reaction command: %v", err),
				})
			}
		}

	}
}

// onInteractionCreate is called when an interaction is created
func (b *Bot) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		cmdName := i.ApplicationCommandData().Name

		cmd, ok := core.GetCommand(cmdName)
		if !ok {
			log.Printf("[WARN] Unknown command: %s\n", cmdName)
			return
		}

		switch i.ApplicationCommandData().CommandType {
		case discordgo.MessageApplicationCommand:
			ctx := &core.MessageApplicationCommandContext{
				Session: s,
				Event:   i,
				Storage: b.storage,
				Target:  i.Message,
			}
			err := cmd.Run(ctx)
			if err != nil {
				log.Println("[ERR] Error running context menu command:", err)
				core.RespondEmbedEphemeral(s, i, &discordgo.MessageEmbed{Description: fmt.Sprintf("Error running context menu command: %v", err)})
			}
		case discordgo.ChatApplicationCommand:
			ctx := &core.SlashInteractionContext{
				Session: s,
				Event:   i,
				Storage: b.storage,
			}
			err := cmd.Run(ctx)
			if err != nil {
				log.Println("[ERR] Error running slash command:", err)
				core.RespondEmbedEphemeral(s, i, &discordgo.MessageEmbed{Description: fmt.Sprintf("Error running slash command: %v", err)})
			}
		}

	case discordgo.InteractionMessageComponent:
		customID := i.MessageComponentData().CustomID
		log.Printf("[DEBUG] Processing component interaction: %s\n", customID)

		var matched core.Command
		for _, cmd := range core.AllCommands() {
			if strings.HasPrefix(customID, cmd.Name()) || strings.HasPrefix(customID, cmd.Name()+":") || strings.HasPrefix(customID, cmd.Name()+"_") {
				matched = cmd
				log.Printf("[DEBUG] Found matching command: %s\n", cmd.Name())
				break
			}
		}

		if matched != nil {
			compHandler, ok := matched.(core.ComponentInteractionHandler)
			if ok {
				log.Printf("[DEBUG] Command %s implements ComponentHandler\n", matched.Name())
				ctx := &core.ComponentInteractionContext{
					Session: s,
					Event:   i,
					Storage: b.storage,
				}
				if err := compHandler.Component(ctx); err != nil {
					log.Printf("[ERR] Error running component command %s: %v\n", matched.Name(), err)
					core.RespondEmbedEphemeral(s, i, &discordgo.MessageEmbed{Description: fmt.Sprintf("Error running component command: %v", err)})
				}
			} else {
				log.Printf("[WARN] Command %s does not implement ComponentHandler interface\n", matched.Name())
				log.Printf("[DEBUG] Command type: %T\n", matched)
			}
		} else {
			log.Printf("[WARN] No matching component for customID: %s\n", customID)
		}

	default:
		log.Printf("[DEBUG] Unknown interaction type: %d\n", i.Type)
	}
}

// registerCommands registers slash commands
func (b *Bot) registerCommands(guildID string) error {
	appID := b.dg.State.User.ID
	if appID == "" {
		user, err := b.dg.User("@me")
		if err != nil {
			return err
		}
		appID = user.ID
	}

	existing, _ := b.dg.ApplicationCommands(appID, guildID)

	var wanted []*discordgo.ApplicationCommand
	wantedNames := make(map[string]bool)

	for _, cmd := range core.AllCommands() {
		if def := normalizeDefinition(cmd); def != nil {
			wanted = append(wanted, def)
			wantedNames[def.Name] = true
		}
	}

	for _, old := range existing {
		if !wantedNames[old.Name] {
			log.Printf("[INFO] Deleting obsolete command: %s\n", old.Name)
			err := b.dg.ApplicationCommandDelete(appID, guildID, old.ID)
			if err != nil {
				log.Printf("[ERR] Failed to delete command %s: %v", old.Name, err)
			}
		}
	}

	toCreate := commandsToUpdate(existing, wanted)
	log.Printf("[INFO] %d commands to register (out of %d)\n", len(toCreate), len(wanted))

	if len(toCreate) == 0 {
		return nil
	}

	registerCommandsWithRateLimit(b, guildID, toCreate)

	return nil
}

// normalizeDefinition normalizes a command definition
func normalizeDefinition(cmd core.Command) *discordgo.ApplicationCommand {
	if slash, ok := cmd.(core.SlashProvider); ok {
		if def := slash.SlashDefinition(); def != nil {
			if def.Type == 0 {
				def.Type = discordgo.ChatApplicationCommand
			}
			return def
		}
	}
	if menu, ok := cmd.(core.ContextMenuProvider); ok {
		if def := menu.ContextDefinition(); def != nil {
			if def.Type == 0 {
				def.Type = discordgo.MessageApplicationCommand
			}
			return def
		}
	}
	return nil
}

// registerCommandsWithRateLimit registers commands with a rate limit
func registerCommandsWithRateLimit(b *Bot, guildID string, cmds []*discordgo.ApplicationCommand) {
	rateLimit := time.Second / 40
	ticker := time.NewTicker(rateLimit)
	defer ticker.Stop()

	var wg sync.WaitGroup

	for _, job := range cmds {
		wg.Add(1)

		go func(cmd *discordgo.ApplicationCommand) {
			defer wg.Done()
			<-ticker.C

			_, err := b.dg.ApplicationCommandCreate(b.dg.State.User.ID, guildID, cmd)
			if err != nil {
				log.Printf("[ERR] Can't create command %s: %v", cmd.Name, err)
			} else {
				log.Printf("[DONE] Command created: %s", cmd.Name)
			}
		}(job)
	}

	wg.Wait()
}

// commandsToUpdate returns commands to update
func commandsToUpdate(existing []*discordgo.ApplicationCommand, wanted []*discordgo.ApplicationCommand) []*discordgo.ApplicationCommand {
	toCreate := make([]*discordgo.ApplicationCommand, 0)

	existingMap := make(map[string]*discordgo.ApplicationCommand)
	for _, e := range existing {
		existingMap[e.Name] = e
	}

	for _, newCmd := range wanted {
		oldCmd, exists := existingMap[newCmd.Name]
		if !exists || !commandsEqual(oldCmd, newCmd) {
			toCreate = append(toCreate, newCmd)
			/*
				fmt.Printf("Command '%s' differs:\n", newCmd.Name)
				spew.Dump(oldCmd)
				fmt.Println("VS:")
				spew.Dump(newCmd)
			*/
		}
	}

	return toCreate
}

// commandsEqual checks if two commands are equal
func commandsEqual(a, b *discordgo.ApplicationCommand) bool {
	if a == nil || b == nil {
		return false
	}

	if a.Name != b.Name || a.Description != b.Description || a.Type != b.Type {
		return false
	}

	if !compareOptions(a.Options, b.Options) {
		return false
	}

	return true
}

// compareOptions checks if two command options are equal
func compareOptions(a, b []*discordgo.ApplicationCommandOption) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i].Name != b[i].Name ||
			a[i].Description != b[i].Description ||
			a[i].Type != b[i].Type ||
			a[i].Required != b[i].Required {
			return false
		}
	}

	return true
}
