package chat

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"server-domme/internal/config"
	"server-domme/internal/core"
	"strings"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

type ChatCommand struct{}

func (c *ChatCommand) Name() string        { return "chat" }
func (c *ChatCommand) Description() string { return "Chat related management commands" }
func (c *ChatCommand) Group() string       { return "chat" }
func (c *ChatCommand) Category() string    { return "💬 Chat" }
func (c *ChatCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

// SlashDefinition with subcommand group "manage" for consistency
func (c *ChatCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "manage",
				Description: "Manage the bot system prompt for this server",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "set",
						Description: "Upload a new system prompt",
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionAttachment,
								Name:        "file",
								Description: "Markdown file (.md) containing the prompt",
								Required:    true,
							},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "reset",
						Description: "Reset the system prompt to default",
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "get",
						Description: "Download the current system prompt",
					},
				},
			},
		},
	}
}

func (c *ChatCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	guildID := e.GuildID

	if !core.IsAdministrator(s, e.Member) {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{Description: "You must be an admin to use this command."})
	}

	if err := core.RespondDeferredEphemeral(s, e); err != nil {
		log.Printf("[ERROR] Failed to defer interaction: %v", err)
		return err
	}

	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	// Subcommand group
	group := data.Options[0]
	if group.Type != discordgo.ApplicationCommandOptionSubCommandGroup || group.Name != "manage" {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Unknown command structure.",
		})
	}

	if len(group.Options) == 0 {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided under manage.",
		})
	}

	sub := group.Options[0]
	switch sub.Name {
	case "set":
		return runSetPrompt(s, e, guildID, sub, &data)
	case "reset":
		return runResetPrompt(s, e, guildID)
	case "get":
		return runGetPrompt(s, e, guildID)
	default:
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
		})
	}
}

func validateMDFile(attachment *discordgo.MessageAttachment, body []byte) error {
	if attachment == nil || !strings.HasSuffix(strings.ToLower(attachment.Filename), ".md") {
		return fmt.Errorf("uploaded file must have `.md` extension")
	}

	if !utf8.Valid(body) {
		return fmt.Errorf("file contains invalid UTF-8 encoding")
	}

	for i, b := range body {
		if b < 0x09 && b != '\n' && b != '\r' && b != '\t' {
			return fmt.Errorf("file contains non-text binary data at byte %d", i)
		}
	}
	return nil
}

func runSetPrompt(s *discordgo.Session, e *discordgo.InteractionCreate, guildID string, sub *discordgo.ApplicationCommandInteractionDataOption, topLevelData *discordgo.ApplicationCommandInteractionData) error {
	if len(sub.Options) == 0 {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No file uploaded.",
		})
	}

	attachmentOption := sub.Options[0]
	attachmentID, ok := attachmentOption.Value.(string)
	if !ok {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to retrieve attachment ID.",
		})
	}

	attachment, exists := topLevelData.Resolved.Attachments[attachmentID]
	if !exists {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to get the uploaded file.",
		})
	}

	resp, err := http.Get(attachment.URL)
	if err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to download the file.",
		})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || len(body) == 0 {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to read the uploaded file or file is empty.",
		})
	}

	if err := validateMDFile(attachment, body); err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Invalid file: %v", err),
		})
	}

	if err := os.MkdirAll("data", 0755); err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to create data directory.",
		})
	}

	path := filepath.Join("data", fmt.Sprintf("%s_chat.prompt.md", guildID))
	if err := os.WriteFile(path, body, 0644); err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to save the uploaded prompt file.",
		})
	}

	return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Successfully uploaded a new system prompt.\nSaved as `%s`", filepath.Base(path)),
	})
}

func runResetPrompt(s *discordgo.Session, e *discordgo.InteractionCreate, guildID string) error {
	path := filepath.Join("data", fmt.Sprintf("%s_chat.prompt.md", guildID))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No custom prompt file found — already using the default.",
		})
	}

	if err := os.Remove(path); err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to remove custom prompt: `%v`", err),
		})
	}

	return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: "Custom system prompt removed. Now using the default prompt.",
	})
}

func runGetPrompt(s *discordgo.Session, e *discordgo.InteractionCreate, guildID string) error {
	localPath := filepath.Join("data", fmt.Sprintf("%s_chat.prompt.md", guildID))
	cfg := config.New()
	globalPath := cfg.AIPromtPath
	if globalPath == "" {
		globalPath = "data/chat.prompt.md"
	}

	var chosenPath string
	if _, err := os.Stat(localPath); err == nil {
		chosenPath = localPath
	} else if _, err := os.Stat(globalPath); err == nil {
		chosenPath = globalPath
	} else {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No system prompt file found (neither guild-specific nor global).",
		})
	}

	file, err := os.Open(chosenPath)
	if err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to open system prompt file.",
		})
	}
	defer file.Close()

	_, err = s.FollowupMessageCreate(e.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("Here's the system prompt for guild `%s`:", guildID),
		Files: []*discordgo.File{
			{
				Name:   filepath.Base(chosenPath),
				Reader: file,
			},
		},
	})
	if err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to send the system prompt file.",
		})
	}
	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&ChatCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
