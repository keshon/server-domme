package media

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"server-domme/internal/core"

	"github.com/bwmarrin/discordgo"
)

type RandomMediaCommand struct{}

func (c *RandomMediaCommand) Name() string        { return "media" }
func (c *RandomMediaCommand) Description() string { return "Post a random NSFW media file" }
func (c *RandomMediaCommand) Group() string       { return "fun" }
func (c *RandomMediaCommand) Category() string    { return "🎞 Media" }
func (c *RandomMediaCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *RandomMediaCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Type:        discordgo.ChatApplicationCommand,
		Options:     []*discordgo.ApplicationCommandOption{},
	}
}

func (c *RandomMediaCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event

	return c.sendMedia(s, e, "")
}

func (c *RandomMediaCommand) sendMedia(s *discordgo.Session, e *discordgo.InteractionCreate, requestedBy string) error {
	file, err := pickRandomFile("./assets/media")
	if err != nil {
		return core.RespondEmbed(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("No media found: %v", err),
		})
	}

	f, err := os.Open(file)
	if err != nil {
		return core.RespondEmbed(s, e, &discordgo.MessageEmbed{
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
			Content: fmt.Sprintf("-# Requested by **%s**", username),

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
							CustomID: "media_next_trigger",
						},
					},
				},
			},
		},
	})
	return err
}

func (c *RandomMediaCommand) Component(ctx *core.ComponentInteractionContext) error {
	e := ctx.Event
	s := ctx.Session

	customID := e.MessageComponentData().CustomID
	log.Printf("[DEBUG] Component handler called for: %s\n", customID)

	if customID != "media_next_trigger" {
		return nil
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

	file, err := pickRandomFile("./assets/media")
	if err != nil {
		_, _ = s.FollowupMessageCreate(e.Interaction, false, &discordgo.WebhookParams{
			Content: fmt.Sprintf("No media found: %v", err),
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
		Content: fmt.Sprintf("-# Requested by **%s**", username),
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
						CustomID: "media_next_trigger",
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

func pickRandomFile(folder string) (string, error) {
	files := []string{}
	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			ext := filepath.Ext(info.Name())
			switch ext {
			case ".mp4", ".webm", ".mov", ".gif", ".jpg", ".png":
				files = append(files, path)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no media files found")
	}

	// return files[rand.Intn(len(files))], nil
	return pickWeightedRandomFile(files), nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&RandomMediaCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
