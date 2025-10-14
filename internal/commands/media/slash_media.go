package media

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
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
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "random",
				Description: "Send a random NSFW media file",
			},
		},
	}
}

func (c *RandomMediaCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	s, e := context.Session, context.Event
	data := e.ApplicationCommandData()
	if len(data.Options) == 0 || data.Options[0].Name != "random" {
		return core.RespondEphemeral(s, e, "Unknown or missing subcommand.")
	}

	return c.sendRandomMedia(s, e)
}

func (c *RandomMediaCommand) sendRandomMedia(s *discordgo.Session, e *discordgo.InteractionCreate) error {
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

	// Send media + button
	err = s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "",
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

// Handle button click for "Next"
func (c *RandomMediaCommand) Component(ctx *core.ComponentInteractionContext) error {
	s := ctx.Session
	e := ctx.Event

	log.Printf("[DEBUG] Component handler called for: %s\n", e.MessageComponentData().CustomID)

	if e.MessageComponentData().CustomID != "media_next_trigger" {
		log.Println("[DEBUG] CustomID doesn't match, returning")
		return nil
	}

	log.Println("[DEBUG] CustomID matches! Proceeding...")

	log.Println("[DEBUG] Acknowledging interaction...")
	err := s.InteractionRespond(e.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("[ERR] Failed to acknowledge interaction: %v\n", err)
		return err
	}

	log.Println("[DEBUG] Picking random file...")
	file, err := pickRandomFile("./assets/media")
	if err != nil {
		log.Printf("[ERR] Failed to pick file: %v\n", err)
		s.FollowupMessageCreate(e.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("❌ No media found: %v", err),
		})
		return err
	}

	log.Printf("[DEBUG] Reading file: %s\n", file)
	fileData, err := os.ReadFile(file)
	if err != nil {
		log.Printf("[ERR] Failed to read file: %v\n", err)
		s.FollowupMessageCreate(e.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("❌ Failed to read media: %v", err),
		})
		return err
	}

	log.Printf("[DEBUG] File size: %d bytes\n", len(fileData))

	log.Println("[DEBUG] Deleting ephemeral response...")
	s.InteractionResponseDelete(e.Interaction)

	log.Println("[DEBUG] Disabling button on original message...")
	_, err = s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:         e.Message.ID,
		Channel:    e.ChannelID,
		Components: &[]discordgo.MessageComponent{},
	})
	if err != nil {
		log.Printf("[ERR] Failed to edit original message: %v\n", err)
	}

	// Send new message with new media
	log.Println("[DEBUG] Sending new message with media...")
	_, err = s.ChannelMessageSendComplex(e.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("🎞 %s", filepath.Base(file)),
		Files: []*discordgo.File{
			{
				Name:   filepath.Base(file),
				Reader: bytes.NewReader(fileData),
			},
		},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    "Next 🔁",
						Style:    discordgo.PrimaryButton,
						CustomID: "media_next_trigger",
					},
				},
			},
		},
	})
	if err != nil {
		log.Printf("[ERR] Failed to send new message: %v\n", err)
	} else {
		log.Println("[DEBUG] Successfully sent new message")
	}

	return err
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

	return files[rand.Intn(len(files))], nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&RandomMediaCommand{},
			core.WithGuildOnly(),
			core.WithGroupAccessCheck(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
