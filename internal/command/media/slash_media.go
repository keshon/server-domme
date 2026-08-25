package media

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/rs/zerolog"
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
	context, ok := ctx.(*cmdadapter.SlashInteractionContext)
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
			context.AppLog.Debug().Str("category", defCat).Str("guild_id", guildID).Msg("media_default_category_used")
		}
	}

	return c.sendMedia(context.AppLog, s, e, guildID, category)
}

func (c *RandomMediaCommand) sendMedia(log zerolog.Logger, s *discordgo.Session, e *discordgo.InteractionCreate, guildID, category string) error {
	// ACK immediately to avoid "Application unavailable" on slow disks/large media sets.
	if err := reply.AckDeferred(s, e); err != nil {
		log.Warn().Err(err).Msg("media_ack_failed")
	}

	baseDir := filepath.Join("assets", "media", guildID)
	searchPath := baseDir

	if category != "" {
		searchPath = filepath.Join(baseDir, category)
	}

	file, err := pickRandomFile(searchPath)
	if err != nil {
		return reply.RespondEmbed(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("No media found in `%s`: %v", categoryOrDefault(category), err),
		})
	}

	f, err := os.Open(file)
	if err != nil {
		return reply.RespondEmbed(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to open media: %v", err),
		})
	}
	defer f.Close()

	username := e.Member.User.Username
	if e.Member.User.GlobalName != "" {
		username = e.Member.User.GlobalName
	}

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
	return err
}

func (c *RandomMediaCommand) Component(ctx *cmdadapter.ComponentInteractionContext) error {
	e := ctx.Event
	s := ctx.Session
	st := ctx.Storage
	guildID := e.GuildID

	customID := e.MessageComponentData().CustomID
	ctx.AppLog.Debug().Str("custom_id", customID).Msg("media_component_received")

	category := ""
	if parts := strings.SplitN(customID, "|", 2); len(parts) == 2 {
		category = parts[1]
	}

	if category == "" && st != nil {
		if defCat, err := st.GetMediaDefault(guildID); err == nil && defCat != "" {
			category = defCat
			ctx.AppLog.Debug().Str("category", defCat).Str("guild_id", guildID).Msg("media_default_category_used")
		}
	}

	err := s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	if err != nil {
		ctx.AppLog.Error().Err(err).Msg("media_ack_failed")
		return err
	}

	username := e.Member.User.Username
	if e.Member.User.GlobalName != "" {
		username = e.Member.User.GlobalName
	}

	file, err := pickRandomFile(filepath.Join("assets", "media", guildID, category))
	if err != nil {
		if _, ferr := s.FollowupMessageCreate(e.Interaction, false, &discordgo.WebhookParams{
			Content: fmt.Sprintf("No media found in `%s`: %v", categoryOrDefault(category), err),
		}); ferr != nil {
			ctx.AppLog.Error().Str("category", category).Err(ferr).Msg("media_followup_failed")
		}
		return nil
	}

	f, err := os.Open(file)
	if err != nil {
		if _, ferr := s.FollowupMessageCreate(e.Interaction, false, &discordgo.WebhookParams{
			Content: fmt.Sprintf("Failed to open media: %v", err),
		}); ferr != nil {
			ctx.AppLog.Error().Str("category", category).Err(ferr).Msg("media_followup_failed")
		}
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
		ctx.AppLog.Error().Str("category", category).Err(err).Msg("media_send_failed")
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
