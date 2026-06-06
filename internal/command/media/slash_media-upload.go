package media

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/command"
	"github.com/keshon/server-domme/internal/discord/discordreply"
	mediastore "github.com/keshon/server-domme/internal/media"
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
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "category",
				Description: "Registered category for the upload (uses default if omitted)",
				Required:    false,
			},
		},
	}
}

func (c *UploadMediaCommand) Run(ctx interface{}) error {
	slashCtx, ok := ctx.(*command.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := slashCtx.Session
	e := slashCtx.Event
	st := slashCtx.Storage
	store := slashCtx.MediaStore
	guildID := e.GuildID

	if err := discordreply.RespondDeferredEphemeral(s, e); err != nil {
		log.Printf("[ERROR] Failed to defer interaction: %v", err)
		return err
	}

	if store == nil {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Media storage is not configured.",
		})
	}

	data := e.ApplicationCommandData()
	options := data.Options

	var requestedCategory string
	files := []*discordgo.MessageAttachment{}

	for _, opt := range options {
		switch opt.Type {
		case discordgo.ApplicationCommandOptionString:
			if opt.Name == "category" {
				requestedCategory = opt.StringValue()
			}
		case discordgo.ApplicationCommandOptionAttachment:
			if att, ok := data.Resolved.Attachments[opt.Value.(string)]; ok {
				files = append(files, att)
			}
		}
	}

	if len(files) == 0 {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No files uploaded.",
		})
	}

	category, err := resolveCategory(st, guildID, requestedCategory)
	if err != nil {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: err.Error(),
		})
	}

	if err := validateRegisteredCategory(st, guildID, category); err != nil {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: err.Error(),
		})
	}

	cmdCtx := context.Background()
	saved := 0
	failed := 0

	for _, file := range files {
		if err := saveUploadedFile(cmdCtx, store, file, guildID, category); err != nil {
			log.Printf("[ERROR] Failed to save uploaded file %s: %v", file.Filename, err)
			failed++
			continue
		}
		saved++
	}

	return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Title: "📥 Media Upload",
		Description: fmt.Sprintf(
			"Saved **%d** file(s) to category `%s` (%d failed)",
			saved, category, failed,
		),
	})
}

func saveUploadedFile(ctx context.Context, store mediastore.Store, att *discordgo.MessageAttachment, guildID, category string) error {
	resp, err := http.Get(att.URL)
	if err != nil {
		return fmt.Errorf("failed to download attachment: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad response downloading file: %v", resp.Status)
	}

	return store.Write(ctx, guildID, category, att.Filename, resp.Body)
}
