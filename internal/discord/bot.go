// /internal/discord/bot.go
package discord

import (
	"context"
	"fmt"
	"log"
	"server-domme/internal/commands"
	"server-domme/internal/storage"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

type Bot struct {
	mu        sync.RWMutex
	dg        *discordgo.Session
	storage   *storage.Storage
	slashCmds map[string][]*discordgo.ApplicationCommand
	logger    *zap.Logger
}

func StartBot(ctx context.Context, token string, storage *storage.Storage, logger *zap.Logger) error {
	b := &Bot{
		storage:   storage,
		slashCmds: make(map[string][]*discordgo.ApplicationCommand),
		logger:    logger,
	}
	if err := b.run(ctx, token); err != nil {
		return fmt.Errorf("bot run error: %w", err)
	}
	return nil
}

func (b *Bot) run(ctx context.Context, token string) error {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	b.dg = dg

	b.configureIntents()
	dg.AddHandler(b.onReady)
	dg.AddHandler(b.onInteractionCreate)

	if err := dg.Open(); err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}
	defer dg.Close()

	fmt.Println("✅ Discord bot is running.")
	<-ctx.Done()
	fmt.Println("❎ Shutdown signal received. Cleaning up...")
	return nil
}

func (b *Bot) configureIntents() {
	b.dg.Identify.Intents = discordgo.IntentsAll
}

func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	botInfo, err := s.User("@me")
	if err != nil {
		log.Println("Warning: Error retrieving bot user:", err)
		return
	}

	for _, g := range r.Guilds {
		if err := b.registerSlashCommands(g.ID); err != nil {
			b.logger.Error("Failed to register slash commands", zap.Error(err))
		}
	}

	fmt.Printf("Bot %v is up and running!\n", botInfo.Username)
}

func (b *Bot) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		cmdName := i.ApplicationCommandData().Name
		args := extractArgs(i)

		if cmd, ok := commands.Get(cmdName); ok && cmd.DCSlashHandler != nil {
			ctx := &commands.SlashContext{
				Session:     s,
				Interaction: i,
				Args:        args,
				Storage:     b.storage,
			}
			cmd.DCSlashHandler(ctx)
		}

	case discordgo.InteractionMessageComponent:
		customID := i.MessageComponentData().CustomID
		parts := strings.SplitN(customID, "_", 2)
		if len(parts) < 1 {
			return
		}
		cmdName := parts[0]
		if cmd, ok := commands.Get(cmdName); ok && cmd.DCComponentHandler != nil {
			ctx := &commands.ComponentContext{
				Session:     s,
				Interaction: i,
				Storage:     b.storage,
			}
			cmd.DCComponentHandler(ctx)
		}

	default:
		fmt.Printf("[DEBUG] Unknown interaction type: %d\n", i.Type)
	}
}

func (b *Bot) registerSlashCommands(guildID string) error {
	var cmds []*discordgo.ApplicationCommand

	for _, cmd := range commands.All() {
		if cmd.DCSlashHandler == nil {
			continue
		}

		slashCmd := &discordgo.ApplicationCommand{
			Name:        cmd.Name,
			Description: cmd.Description,
			Options:     cmd.SlashOptions,
		}

		cmds = append(cmds, slashCmd)
	}

	var created []*discordgo.ApplicationCommand
	for _, cmd := range cmds {
		c, err := b.dg.ApplicationCommandCreate(b.dg.State.User.ID, guildID, cmd)
		if err != nil {
			return fmt.Errorf("register command %s: %w", cmd.Name, err)
		}
		created = append(created, c)
	}

	b.mu.Lock()
	b.slashCmds[guildID] = created
	b.mu.Unlock()

	return nil
}

func splitArgs(input string) []string {
	var args []string
	var current strings.Builder
	inQuotes := false
	escapeNext := false

	for _, r := range input {
		switch {
		case escapeNext:
			current.WriteRune(r)
			escapeNext = false
		case r == '\\':
			escapeNext = true
		case r == '"':
			inQuotes = !inQuotes
		case r == ' ' && !inQuotes:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

func extractArgs(i *discordgo.InteractionCreate) []string {
	if i.Type != discordgo.InteractionApplicationCommand {
		return nil
	}
	options := i.ApplicationCommandData().Options
	var args []string
	for _, opt := range options {
		switch opt.Type {
		case discordgo.ApplicationCommandOptionSubCommand, discordgo.ApplicationCommandOptionSubCommandGroup:
			args = append(args, opt.Name)
			for _, subOpt := range opt.Options {
				args = append(args, fmt.Sprintf("%v", subOpt.Value))
			}
		default:
			args = append(args, fmt.Sprintf("%v", opt.Value))
		}
	}
	return args
}
