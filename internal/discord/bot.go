package discord

import (
	"context"
	"fmt"
	"log"
	"server-domme/internal/bot"
	"server-domme/internal/command"
	"server-domme/internal/config"

	"server-domme/internal/music/player"
	"server-domme/internal/music/source_resolver"
	"server-domme/internal/storage"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Bot is a Discord bot
type Bot struct {
	dg             *discordgo.Session
	storage        *storage.Storage
	slashCmds      map[string][]*discordgo.ApplicationCommand
	cfg            *config.Config
	mu             sync.RWMutex
	sourceResolver *source_resolver.SourceResolver
	players        map[string]*player.Player
}

// StartBot starts the Discord bot
func StartBot(ctx context.Context, cfg *config.Config, storage *storage.Storage) error {
	b := &Bot{
		cfg:       cfg,
		storage:   storage,
		slashCmds: make(map[string][]*discordgo.ApplicationCommand),
		players:   make(map[string]*player.Player),
	}
	if err := b.run(ctx, cfg.DiscordToken); err != nil {
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

	go func() {
		for evt := range bot.SystemEvents() {
			switch evt.Type {
			case bot.SystemEventRefreshCommands:
				go b.handleRefreshCommands(evt)
			}
		}
	}()

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

	for _, cmd := range command.AllCommands() {
		ctx := &command.MessageContext{
			Session: s,
			Event:   m,
			Storage: b.storage,
		}
		err := cmd.Run(ctx)
		if err != nil {
			log.Println("[ERR] Error running command:", err)
			bot.MessageEmbed(s, m.ChannelID, &discordgo.MessageEmbed{
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

	// Leave any blacklisted guilds on startup
	for _, g := range r.Guilds {
		if b.isGuildBlacklisted(g.ID) {
			log.Printf("[INFO] Leaving blacklisted guild: %s (%s)", g.ID, g.Name)
			if err := s.GuildLeave(g.ID); err != nil {
				log.Printf("[ERR] Failed to leave guild %s: %v", g.ID, err)
			}
			continue
		}

		b.registerMusicCommands()

		if b.cfg.InitSlashCommands {
			if err := b.registerCommands(g.ID); err != nil {
				log.Println("[ERR] Error registering slash commands for guild", g.ID, ":", err)
			}
		} else {
			log.Println("[INFO] Registering slash commands skipped")
		}
	}

	log.Println("[INFO] Starting commands services...")
	purgeScheduler(b.storage, s)
	go shortlinkServer(b.storage)

	if err := updateReadme(); err != nil {
		log.Println("[ERR] Failed to update README:", err)
	}

	log.Printf("[INFO] ✅ Discord bot %v is running.", botInfo.Username)
}

// onGuildCreate is called when a guild is created
func (b *Bot) onGuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	log.Printf("[INFO] Bot added to guild: %s (%s)", g.Guild.ID, g.Guild.Name)

	if b.isGuildBlacklisted(g.Guild.ID) {
		log.Printf("[INFO] Leaving blacklisted guild: %s (%s)", g.Guild.ID, g.Guild.Name)
		if err := s.GuildLeave(g.Guild.ID); err != nil {
			log.Printf("[ERR] Failed to leave guild %s: %v", g.Guild.ID, err)
		}
		return
	}

	b.registerMusicCommands()

	if err := b.registerCommands(g.Guild.ID); err != nil {
		log.Printf("[ERR] Failed to register commands for new guild %s: %v", g.Guild.ID, err)
	}
}

// onMessageReactionAdd is called when a reaction is added
func (b *Bot) onMessageReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	for _, cmd := range command.AllCommands() {
		if _, ok := cmd.(command.ReactionProvider); ok {
			ctx := &command.MessageReactionContext{
				Session: s,
				Event:   r,
				Storage: b.storage,
			}
			err := cmd.Run(ctx)
			if err != nil {
				log.Println("[ERR] Error running reaction command:", err)
				bot.MessageEmbed(s, r.ChannelID, &discordgo.MessageEmbed{
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

		cmd, ok := command.GetCommand(cmdName)
		if !ok {
			log.Printf("[WARN] Unknown command: %s\n", cmdName)
			return
		}

		switch i.ApplicationCommandData().CommandType {
		case discordgo.MessageApplicationCommand:
			ctx := &command.MessageApplicationCommandContext{
				Session: s,
				Event:   i,
				Storage: b.storage,
				Target:  i.Message,
			}
			err := cmd.Run(ctx)
			if err != nil {
				log.Println("[ERR] Error running context menu command:", err)
				bot.RespondEmbedEphemeral(s, i, &discordgo.MessageEmbed{Description: fmt.Sprintf("Error running context menu command: %v", err)})
			}
		case discordgo.ChatApplicationCommand:
			ctx := &command.SlashInteractionContext{
				Session: s,
				Event:   i,
				Storage: b.storage,
			}
			err := cmd.Run(ctx)
			if err != nil {
				log.Println("[ERR] Error running slash command:", err)
				bot.RespondEmbedEphemeral(s, i, &discordgo.MessageEmbed{Description: fmt.Sprintf("Error running slash command: %v", err)})
			}
		}

	case discordgo.InteractionMessageComponent:
		customID := i.MessageComponentData().CustomID
		log.Printf("[DEBUG] Processing component interaction: %s\n", customID)

		var matched command.Command
		for _, cmd := range command.AllCommands() {
			if strings.HasPrefix(customID, cmd.Name()) || strings.HasPrefix(customID, cmd.Name()+":") || strings.HasPrefix(customID, cmd.Name()+"_") {
				matched = cmd
				log.Printf("[DEBUG] Found matching command: %s\n", cmd.Name())
				break
			}
		}

		if matched != nil {
			log.Printf("[DEBUG] Matched command type: %T", matched)
			compHandler, ok := matched.(command.ComponentInteractionHandler)
			log.Printf("[DEBUG] ComponentInteractionHandler? %v", ok)
			if ok {
				log.Printf("[DEBUG] Command %s implements ComponentHandler\n", matched.Name())
				log.Printf("[DEBUG] About to call Component() method...\n")
				ctx := &command.ComponentInteractionContext{
					Session: s,
					Event:   i,
					Storage: b.storage,
				}
				err := compHandler.Component(ctx)
				if err != nil {
					log.Printf("[ERR] Error running component command %s: %v\n", matched.Name(), err)
					bot.RespondEmbedEphemeral(s, i, &discordgo.MessageEmbed{Description: fmt.Sprintf("Error running component command: %v", err)})
				}
				log.Printf("[DEBUG] Component() method completed: %s\n", matched.Name())
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
	localHashes := loadGuildCommandHashes(guildID)

	var wanted []*discordgo.ApplicationCommand
	wantedHashes := make(map[string]string)
	for _, cmd := range command.AllCommands() {
		if def := normalizeDefinition(cmd); def != nil {
			wanted = append(wanted, def)
			wantedHashes[def.Name] = hashCommand(def)
		}
	}

	// Delete obsolete
	for _, old := range existing {
		if _, ok := wantedHashes[old.Name]; !ok {
			log.Printf("[INFO] [%s] Deleting obsolete command: %s", guildID, old.Name)
			if err := b.dg.ApplicationCommandDelete(appID, guildID, old.ID); err != nil {
				log.Printf("[ERR] [%s] Failed to delete %s: %v", guildID, old.Name, err)
			}
			delete(localHashes, old.Name)
		}
	}

	// Create or update changed commands
	var changed []*discordgo.ApplicationCommand
	for _, cmd := range wanted {
		newHash := wantedHashes[cmd.Name]
		if localHashes[cmd.Name] != newHash {
			changed = append(changed, cmd)
		}
	}

	if len(changed) > 0 {
		log.Printf("[INFO] [%s] %d commands changed — updating with rate limit...", guildID, len(changed))
		registerCommandsWithRateLimit(b, guildID, changed)
		for _, c := range changed {
			localHashes[c.Name] = wantedHashes[c.Name]
		}
	}

	saveGuildCommandHashes(guildID, localHashes)
	return nil
}

func (b *Bot) isGuildBlacklisted(guildID string) bool {
	return slices.Contains(b.cfg.DiscordGuildBlacklist, guildID)
}

// normalizeDefinition normalizes a command definition
func normalizeDefinition(cmd command.Command) *discordgo.ApplicationCommand {
	if slash, ok := cmd.(command.SlashProvider); ok {
		if def := slash.SlashDefinition(); def != nil {
			if def.Type == 0 {
				def.Type = discordgo.ChatApplicationCommand
			}
			return def
		}
	}
	if menu, ok := cmd.(command.ContextMenuProvider); ok {
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

func (b *Bot) handleRefreshCommands(evt bot.SystemEvent) {
	if b.isGuildBlacklisted(evt.GuildID) {
		log.Printf("[SKIP][%s] Guild is blacklisted, skipping refresh.", evt.GuildID)
		return
	}

	log.Printf("[REFRESH][%s] Re-registering slash commands (target=%s)...", evt.GuildID, evt.Target)

	appID := b.dg.State.User.ID
	if appID == "" {
		user, err := b.dg.User("@me")
		if err != nil {
			log.Printf("[ERR][%s] Failed to fetch self: %v", evt.GuildID, err)
			return
		}
		appID = user.ID
	}

	if strings.ToLower(evt.Target) == "all" || evt.Target == "" {
		err := b.registerCommands(evt.GuildID)
		if err != nil {
			log.Printf("[ERR][%s] Refresh failed: %v", evt.GuildID, err)
			return
		}
		log.Printf("[DONE][%s] All commands refreshed.", evt.GuildID)
		return
	}

	for _, cmd := range command.AllCommands() {
		if strings.EqualFold(cmd.Name(), evt.Target) {
			if def := normalizeDefinition(cmd); def != nil {
				_, err := b.dg.ApplicationCommandCreate(appID, evt.GuildID, def)
				if err != nil {
					log.Printf("[ERR][%s] Failed to refresh command '%s': %v", evt.GuildID, def.Name, err)
					return
				}
				log.Printf("[DONE][%s] Command '%s' refreshed.", evt.GuildID, def.Name)
				return
			}
		}
	}

	log.Printf("[WARN][%s] No command named '%s' found.", evt.GuildID, evt.Target)
}
