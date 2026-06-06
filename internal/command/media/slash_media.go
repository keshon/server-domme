package media

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/command"
	"github.com/keshon/server-domme/internal/discord/discordreply"
	mediastore "github.com/keshon/server-domme/internal/media"
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
				Description: "Optional category to pull from (if omitted, uses default or all)",
				Required:    false,
			},
		},
	}
}

func (c *RandomMediaCommand) Run(ctx interface{}) error {
	slashCtx, ok := ctx.(*command.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := slashCtx.Session
	e := slashCtx.Event
	st := slashCtx.Storage
	store := slashCtx.MediaStore
	guildID := e.GuildID

	data := e.ApplicationCommandData()
	var requested string
	if len(data.Options) > 0 {
		requested = data.Options[0].StringValue()
	}

	category, err := resolveCategory(st, guildID, requested)
	if err != nil && requested == "" {
		// No category and no default — search entire guild.
		category = ""
	} else if err != nil {
		return discordreply.RespondEmbed(s, e, &discordgo.MessageEmbed{
			Description: err.Error(),
		})
	}

	if category != "" {
		log.Printf("[INFO] Using media category '%s' for guild %s", category, guildID)
	}

	return c.sendMedia(context.Background(), s, e, store, guildID, category)
}

func (c *RandomMediaCommand) sendMedia(ctx context.Context, s *discordgo.Session, e *discordgo.InteractionCreate, store mediastore.Store, guildID, category string) error {
	if store == nil {
		return discordreply.RespondEmbed(s, e, &discordgo.MessageEmbed{
			Description: "Media storage is not configured.",
		})
	}

	if err := discordreply.AckDeferred(s, e); err != nil {
		log.Printf("[WARN] media: failed to ACK deferred: %v", err)
	}

	file, err := pickRandomFile(ctx, store, guildID, category)
	if err != nil {
		return discordreply.RespondEmbed(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("No media found in `%s`: %v", categoryOrDefault(category), err),
		})
	}

	reader, err := store.Read(ctx, file.Path)
	if err != nil {
		log.Printf("[WARN] media: read failed path=%q: %v", file.Path, err)
		return discordreply.RespondEmbed(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to open media: %v", err),
		})
	}
	defer reader.Close()

	username := e.Member.User.Username
	if e.Member.User.GlobalName != "" {
		username = e.Member.User.GlobalName
	}

	_, err = s.FollowupMessageCreate(e.Interaction, false, &discordgo.WebhookParams{
		Content: fmt.Sprintf("`#%s`\n-# Requested by **%s**", categoryOrDefault(category), username),
		Files: []*discordgo.File{{
			Name:   file.Name,
			Reader: reader,
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

func (c *RandomMediaCommand) Component(ctx *command.ComponentInteractionContext) error {
	e := ctx.Event
	s := ctx.Session
	st := ctx.Storage
	store := ctx.MediaStore
	guildID := e.GuildID

	customID := e.MessageComponentData().CustomID
	log.Printf("[DEBUG] Component handler called for: %s\n", customID)

	requested := ""
	if parts := strings.SplitN(customID, "|", 2); len(parts) == 2 {
		requested = parts[1]
	}

	category, err := resolveCategory(st, guildID, requested)
	if err != nil && requested == "" {
		category = ""
	} else if err != nil {
		return s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: err.Error(),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}

	if store == nil {
		return s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Media storage is not configured.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
	}

	if err := s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); err != nil {
		log.Println("[ERR] Failed to ACK interaction:", err)
		return err
	}

	username := e.Member.User.Username
	if e.Member.User.GlobalName != "" {
		username = e.Member.User.GlobalName
	}

	cmdCtx := context.Background()
	file, err := pickRandomFile(cmdCtx, store, guildID, category)
	if err != nil {
		if _, ferr := s.FollowupMessageCreate(e.Interaction, false, &discordgo.WebhookParams{
			Content: fmt.Sprintf("No media found in `%s`: %v", categoryOrDefault(category), err),
		}); ferr != nil {
			log.Printf("[ERR] Failed to send media followup (no media): %v", ferr)
		}
		return nil
	}

	reader, err := store.Read(cmdCtx, file.Path)
	if err != nil {
		if _, ferr := s.FollowupMessageCreate(e.Interaction, false, &discordgo.WebhookParams{
			Content: fmt.Sprintf("Failed to open media: %v", err),
		}); ferr != nil {
			log.Printf("[ERR] Failed to send media followup (open failed): %v", ferr)
		}
		return nil
	}
	defer reader.Close()

	_, err = s.FollowupMessageCreate(e.Interaction, false, &discordgo.WebhookParams{
		Content: fmt.Sprintf("`#%s`\n-# Requested by **%s**", categoryOrDefault(category), username),
		Files: []*discordgo.File{{
			Name:   file.Name,
			Reader: reader,
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

// --- Weighted random system ---
var (
	recentHistory   = []string{}
	historyLimit    = 20
	recencyDecay    = 0.5
	recentHistoryMu sync.Mutex
)

func pickRandomFile(ctx context.Context, store mediastore.Store, guildID, category string) (mediastore.File, error) {
	files, err := store.List(ctx, guildID, category)
	if err != nil {
		return mediastore.File{}, err
	}
	if len(files) == 0 {
		return mediastore.File{}, fmt.Errorf("no files found")
	}
	return pickWeightedRandomFile(files), nil
}

func pickWeightedRandomFile(files []mediastore.File) mediastore.File {
	recentHistoryMu.Lock()
	defer recentHistoryMu.Unlock()

	if len(files) == 0 {
		return mediastore.File{}
	}
	if len(files) == 1 {
		updateHistory(files[0].Path)
		return files[0]
	}

	weights := make([]float64, len(files))
	for i, file := range files {
		recencyIndex := findInHistory(file.Path)
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
			updateHistory(files[i].Path)
			return files[i]
		}
	}

	updateHistory(files[len(files)-1].Path)
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
