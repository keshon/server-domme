package media

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"server-domme/internal/bot"
	"server-domme/internal/command"
	"server-domme/internal/middleware"

	"github.com/bwmarrin/discordgo"
)

type UploadMediaCommand struct{}

func (c *UploadMediaCommand) Name() string        { return "upload-media" }
func (c *UploadMediaCommand) Description() string { return "Upload one or multiple media files" }
func (c *UploadMediaCommand) Group() string       { return "media" }
func (c *UploadMediaCommand) Category() string    { return "🎞️ Media" }
func (c *UploadMediaCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func (c *UploadMediaCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        "file1",
				Description: "Upload a media file (image/video/etc)",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        "file2",
				Description: "Optional 2nd file",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        "file3",
				Description: "Optional 3rd file",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        "file4",
				Description: "Optional 4th file",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        "file5",
				Description: "Optional 5th file",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        "file6",
				Description: "Optional 6th file",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        "file7",
				Description: "Optional 7th file",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        "file8",
				Description: "Optional 8th file",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        "file9",
				Description: "Optional 9th file",
				Required:    false,
			},
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        "file10",
				Description: "Optional 10th file",
				Required:    false,
			},
			// ✅ Optional string goes last
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "category",
				Description: "Tag or category for the uploaded media (e.g. memes, cats)",
				Required:    false,
			},
		},
	}
}

func (c *UploadMediaCommand) Run(ctx interface{}) error {
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
	options := data.Options

	var category string = "uncategorized"
	files := []*discordgo.MessageAttachment{}

	// Extract category + attachments
	for _, opt := range options {
		switch opt.Type {
		case discordgo.ApplicationCommandOptionString:
			if opt.Name == "category" {
				category = sanitizeCategory(opt.StringValue())
			}
		case discordgo.ApplicationCommandOptionAttachment:
			if att, ok := data.Resolved.Attachments[opt.Value.(string)]; ok {
				files = append(files, att)
			}
		}
	}

	if len(files) == 0 {
		return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No files uploaded.",
		})
	}

	saved := 0
	failed := 0

	for _, file := range files {
		if err := saveUploadedFile(file, guildID, category); err != nil {
			log.Printf("[ERROR] Failed to save uploaded file %s: %v", file.Filename, err)
			failed++
			continue
		}
		saved++
	}

	return bot.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Title: "📥 Media Upload",
		Description: fmt.Sprintf(
			"Saved **%d** file(s) to category `%s` (%d failed)",
			saved, category, failed,
		),
	})
}

func sanitizeCategory(cat string) string {
	cat = strings.TrimSpace(cat)
	if cat == "" {
		return "uncategorized"
	}
	cat = strings.ToLower(cat)
	cat = strings.ReplaceAll(cat, " ", "_")
	return cat
}

func saveUploadedFile(att *discordgo.MessageAttachment, guildID, category string) error {
	resp, err := http.Get(att.URL)
	if err != nil {
		return fmt.Errorf("failed to download attachment: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad response downloading file: %v", resp.Status)
	}

	dir := filepath.Join("assets", "media", guildID, category)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create dir: %v", err)
	}

	destPath := filepath.Join(dir, att.Filename)
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}

	return nil
}

func init() {
	command.RegisterCommand(
		&UploadMediaCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
}
