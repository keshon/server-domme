package chat

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"server-domme/internal/bot"
	"server-domme/internal/command"
	"server-domme/internal/config"
	"server-domme/internal/middleware"

	"strings"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

type ManageChatCommand struct{}

func (c *ManageChatCommand) Name() string        { return "manage-chat" }
func (c *ManageChatCommand) Description() string { return "Chat settings" }
func (c *ManageChatCommand) Group() string       { return "chat" }
func (c *ManageChatCommand) Category() string    { return "⚙️ Settings" }
func (c *ManageChatCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func (c *ManageChatCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "upload-prompt",
				Description: "Upload a new system prompt for this server",
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
				Name:        "download-prompt",
				Description: "Download the current system prompt for this server",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "reset-prompt",
				Description: "Reset the system prompt to default for this server",
			},
		},
	}
}

func (c *ManageChatCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*command.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	guildID := e.GuildID

	if err := bot.RespondDeferredEphemeral(s, e); err != nil {
		log.Printf("[ERROR] Failed to defer interaction: %v", err)
		return err
	}

	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	sub := data.Options[0]
	switch sub.Name {
	case "upload-prompt":
		return c.runSetPrompt(s, e, guildID, sub, &data)
	case "download-prompt":
		return c.runGetPrompt(s, e, guildID)
	case "reset-prompt":
		return c.runResetPrompt(s, e, guildID)
	default:
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
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

func (c *ManageChatCommand) runSetPrompt(s *discordgo.Session, e *discordgo.InteractionCreate, guildID string, sub *discordgo.ApplicationCommandInteractionDataOption, topLevelData *discordgo.ApplicationCommandInteractionData) error {
	if len(sub.Options) == 0 {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No file uploaded.",
		})
	}

	attachmentOption := sub.Options[0]
	attachmentID, ok := attachmentOption.Value.(string)
	if !ok {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to retrieve attachment ID.",
		})
	}

	attachment, exists := topLevelData.Resolved.Attachments[attachmentID]
	if !exists {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to get the uploaded file.",
		})
	}

	resp, err := http.Get(attachment.URL)
	if err != nil {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to download the file.",
		})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || len(body) == 0 {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to read the uploaded file or file is empty.",
		})
	}

	if err := validateMDFile(attachment, body); err != nil {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Invalid file: %v", err),
		})
	}

	if err := os.MkdirAll("data", 0755); err != nil {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to create data directory.",
		})
	}

	path := filepath.Join("data", fmt.Sprintf("%s_chat.prompt.md", guildID))
	if err := os.WriteFile(path, body, 0644); err != nil {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to save the uploaded prompt file.",
		})
	}

	return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Successfully uploaded a new system prompt.\nSaved as `%s`", filepath.Base(path)),
	})
}

func (c *ManageChatCommand) runResetPrompt(s *discordgo.Session, e *discordgo.InteractionCreate, guildID string) error {
	path := filepath.Join("data", fmt.Sprintf("%s_chat.prompt.md", guildID))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No custom prompt file found — already using the default.",
		})
	}

	if err := os.Remove(path); err != nil {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to remove custom prompt: `%v`", err),
		})
	}

	return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: "Custom system prompt removed. Now using the default prompt.",
	})
}

func (c *ManageChatCommand) runGetPrompt(s *discordgo.Session, e *discordgo.InteractionCreate, guildID string) error {
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
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No system prompt file found (neither guild-specific nor global).",
		})
	}

	file, err := os.Open(chosenPath)
	if err != nil {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
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
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to send the system prompt file.",
		})
	}
	return nil
}

func init() {
	command.RegisterCommand(
		command.ApplyMiddlewares(
			&ManageChatCommand{},
			middleware.WithGroupAccessCheck(),
			middleware.WithGuildOnly(),
			middleware.WithUserPermissionCheck(),
			middleware.WithCommandLogger(),
		),
	)
}
