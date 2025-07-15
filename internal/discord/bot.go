// /internal/discord/bot.go
package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"server-domme/internal/commands"
	"server-domme/internal/storage"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	mu        sync.RWMutex
	dg        *discordgo.Session
	storage   *storage.Storage
	slashCmds map[string][]*discordgo.ApplicationCommand
}

func StartBot(ctx context.Context, token string, storage *storage.Storage) error {
	b := &Bot{
		storage:   storage,
		slashCmds: make(map[string][]*discordgo.ApplicationCommand),
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

	log.Println("✅ Discord bot is running.")
	<-ctx.Done()
	log.Println("❎ Shutdown signal received. Cleaning up...")
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
			log.Println("Error registering slash commands for guild", g.ID, ":", err)
		}
	}

	log.Printf("Bot %v is up and running!\n", botInfo.Username)

	go startScheduledNukes(b.storage, s)
}

func (b *Bot) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		cmdName := i.ApplicationCommandData().Name
		args := extractArgs(i)

		if cmd, ok := commands.Get(cmdName); ok && cmd.DCSlashHandler != nil {
			ctx := &commands.SlashContext{
				Session:           s,
				InteractionCreate: i,
				Args:              args,
				Storage:           b.storage,
			}
			cmd.DCSlashHandler(ctx)
		}

	case discordgo.InteractionMessageComponent:
		customID := i.MessageComponentData().CustomID

		var matchedCommand *commands.Command
		for _, cmd := range commands.All() {
			if strings.HasPrefix(customID, cmd.Name) || strings.HasPrefix(customID, cmd.Name+":") || strings.HasPrefix(customID, cmd.Name+"_") {
				matchedCommand = cmd
				break
			}
		}

		if matchedCommand != nil && matchedCommand.DCComponentHandler != nil {
			ctx := &commands.ComponentContext{
				Session:           s,
				InteractionCreate: i,
				Storage:           b.storage,
			}
			matchedCommand.DCComponentHandler(ctx)
		} else {
			log.Printf("[WARN] No matching command handler for CustomID: %s\n", customID)
		}

	default:
		log.Printf("[DEBUG] Unknown interaction type: %d\n", i.Type)
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

func startScheduledNukes(st *storage.Storage, session *discordgo.Session) {
	log.Println("Starting scheduled nukes...")

	records := st.GetRecordsList()

	for _, data := range records {
		jsonData, _ := json.Marshal(data)
		var rec storage.Record
		err := json.Unmarshal(jsonData, &rec)
		if err != nil {
			log.Printf("Error unmarshalling to *Record: %v", err)
			continue
		}

		for _, job := range rec.NukeJobs {
			log.Printf("Found nuke job — Mode: %s | Guild: %s | Channel: %s", job.Mode, job.GuildID, job.ChannelID)

			switch job.Mode {
			case "delayed":
				dur := time.Until(job.DelayUntil)
				if dur > 0 {
					log.Printf("Scheduling delayed nuke in %v for channel %s", dur, job.ChannelID)

					go func(job storage.NukeJob) {
						time.Sleep(dur)
						log.Printf("Executing delayed nuke for channel %s", job.ChannelID)

						commands.DeleteMessages(session, job.ChannelID, nil, nil, nil)

						rec.NukeJobs[job.ChannelID] = job
						st.SetNukeJob(job.GuildID, job.ChannelID, job.Mode, job.DelayUntil, job.Silent)

						log.Printf("Delayed nuke complete for channel %s", job.ChannelID)
					}(job)
				} else {
					log.Printf("Skipping expired delayed job for channel %s (DelayUntil in the past)", job.ChannelID)
				}

			case "recurring":
				dur, err := time.ParseDuration(job.OlderThan)
				if err != nil {
					log.Printf("Failed to parse OlderThan duration '%s' for channel %s: %v", job.OlderThan, job.ChannelID, err)
					continue
				}

				stopChan := make(chan struct{})
				commands.ActiveDeletionsMu.Lock()
				commands.ActiveDeletions[job.ChannelID] = stopChan
				commands.ActiveDeletionsMu.Unlock()

				log.Printf("Starting recurring nuke for channel %s every 30s (older than %v)", job.ChannelID, dur)

				go func(job storage.NukeJob, d time.Duration) {
					ticker := time.NewTicker(30 * time.Second)
					defer ticker.Stop()

					for {
						select {
						case <-stopChan:
							log.Printf("Stopping recurring nuke for channel %s", job.ChannelID)
							return
						case <-ticker.C:
							start := time.Now().Add(-d)
							now := time.Now()
							log.Printf("Recurring nuke triggered for channel %s | Window: %v – %v", job.ChannelID, start, now)
							commands.DeleteMessages(session, job.ChannelID, &start, &now, stopChan)
						}
					}
				}(job, dur)
			default:
				log.Printf("Unknown nuke mode '%s' for channel %s", job.Mode, job.ChannelID)
			}
		}
	}
}
