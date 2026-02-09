package media

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"server-domme/internal/bot"
	"server-domme/internal/command"
	"server-domme/internal/middleware"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

type RandomMediaCommand struct{}

func (c *RandomMediaCommand) Name() string        { return "media" }
func (c *RandomMediaCommand) Description() string { return "Post a random media file" }
func (c *RandomMediaCommand) Group() string       { return "media" }
func (c *RandomMediaCommand) Category() string    { return "🎞️ Media" }
func (c *RandomMediaCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *RandomMediaCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Type:        discordgo.ChatApplicationCommand,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "category",
				Description: "Optional category to pull from (if omitted, uses default or random)",
				Required:    false,
			},
		},
	}
}

func (c *RandomMediaCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*command.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	st := context.Storage
	guildID := e.GuildID

	data := e.ApplicationCommandData()
	var category string

	if len(data.Options) > 0 {
		category = data.Options[0].StringValue()
	}

	if category == "" && st != nil {
		if defCat, err := st.GetMediaDefault(guildID); err == nil && defCat != "" {
			category = defCat
			log.Printf("[INFO] Using default media category '%s' for guild %s", defCat, guildID)
		}
	}

	return c.sendMedia(s, e, guildID, category)
}

func (c *RandomMediaCommand) sendMedia(s *discordgo.Session, e *discordgo.InteractionCreate, guildID, category string) error {
	baseDir := filepath.Join("assets", "media", guildID)
	searchPath := baseDir

	if category != "" {
		searchPath = filepath.Join(baseDir, category)
	}

	file, err := pickRandomFile(searchPath)
	if err != nil {
		return bot.RespondEmbed(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("No media found in `%s`: %v", categoryOrDefault(category), err),
		})
	}

	f, err := os.Open(file)
	if err != nil {
		return bot.RespondEmbed(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to open media: %v", err),
		})
	}
	defer f.Close()

	username := e.Member.User.Username
	if e.Member.User.GlobalName != "" {
		username = e.Member.User.GlobalName
	}

	err = s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("`#%s`\n-# Requested by **%s**", categoryOrDefault(category), username),
			Files: []*discordgo.File{{
				Name:   filepath.Base(file),
				Reader: f,
			}},
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Next",
							Style:    discordgo.SecondaryButton,
							CustomID: fmt.Sprintf("media_next_trigger|%s", category),
						},
					},
				},
			},
		},
	})
	return err
}

func (c *RandomMediaCommand) Component(ctx *command.ComponentInteractionContext) error {
	e := ctx.Event
	s := ctx.Session
	st := ctx.Storage
	guildID := e.GuildID

	customID := e.MessageComponentData().CustomID
	log.Printf("[DEBUG] Component handler called for: %s\n", customID)

	category := ""
	if parts := strings.SplitN(customID, "|", 2); len(parts) == 2 {
		category = parts[1]
	}

	if category == "" && st != nil {
		if defCat, err := st.GetMediaDefault(guildID); err == nil && defCat != "" {
			category = defCat
			log.Printf("[INFO] Using default media category '%s' for follow-up in guild %s", defCat, guildID)
		}
	}

	err := s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	if err != nil {
		log.Println("[ERR] Failed to ACK interaction:", err)
		return err
	}

	username := e.Member.User.Username
	if e.Member.User.GlobalName != "" {
		username = e.Member.User.GlobalName
	}

	file, err := pickRandomFile(filepath.Join("assets", "media", guildID, category))
	if err != nil {
		_, _ = s.FollowupMessageCreate(e.Interaction, false, &discordgo.WebhookParams{
			Content: fmt.Sprintf("No media found in `%s`: %v", categoryOrDefault(category), err),
		})
		return nil
	}

	f, err := os.Open(file)
	if err != nil {
		_, _ = s.FollowupMessageCreate(e.Interaction, false, &discordgo.WebhookParams{
			Content: fmt.Sprintf("Failed to open media: %v", err),
		})
		return nil
	}
	defer f.Close()

	_, err = s.FollowupMessageCreate(e.Interaction, false, &discordgo.WebhookParams{
		Content: fmt.Sprintf("`#%s`\n-# Requested by **%s**", categoryOrDefault(category), username),
		Files: []*discordgo.File{{
			Name:   filepath.Base(file),
			Reader: f,
		}},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    "Next",
						Style:    discordgo.SecondaryButton,
						CustomID: fmt.Sprintf("media_next_trigger|%s", category),
					},
				},
			},
		},
	})
	if err != nil {
		log.Println("[ERR] Failed to send follow-up media:", err)
	}
	return nil
}

func categoryOrDefault(cat string) string {
	if cat == "" {
		return "random"
	}
	return cat
}

// --- Weighted random system ---
var (
	recentHistory   = []string{}
	historyLimit    = 20
	recencyDecay    = 0.5
	recentHistoryMu sync.Mutex
)

func pickRandomFile(root string) (string, error) {
	files := []string{}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no files found")
	}

	return pickWeightedRandomFile(files), nil
}

func pickWeightedRandomFile(files []string) string {
	recentHistoryMu.Lock()
	defer recentHistoryMu.Unlock()

	if len(files) == 0 {
		return ""
	}
	if len(files) == 1 {
		updateHistory(files[0])
		return files[0]
	}

	weights := make([]float64, len(files))
	for i, file := range files {
		recencyIndex := findInHistory(file)
		if recencyIndex == -1 {
			weights[i] = 1.0
		} else {
			positionFromEnd := len(recentHistory) - recencyIndex - 1
			weights[i] = math.Exp(-recencyDecay * float64(positionFromEnd))
		}
	}

	total := 0.0
	for _, w := range weights {
		total += w
	}

	r := rand.Float64() * total
	acc := 0.0
	for i, w := range weights {
		acc += w
		if r <= acc {
			updateHistory(files[i])
			return files[i]
		}
	}

	updateHistory(files[len(files)-1])
	return files[len(files)-1]
}

func findInHistory(file string) int {
	for i, f := range recentHistory {
		if f == file {
			return i
		}
	}
	return -1
}

func updateHistory(file string) {
	recentHistory = append(recentHistory, file)
	if len(recentHistory) > historyLimit {
		recentHistory = recentHistory[len(recentHistory)-historyLimit:]
	}
}

func init() {
	command.RegisterCommand(
		&RandomMediaCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
}
