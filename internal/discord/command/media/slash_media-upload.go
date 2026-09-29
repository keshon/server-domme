package media

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/perm"
	"github.com/keshon/server-domme/internal/discord/reply"
)

type UploadMediaCommand struct{}

func (c *UploadMediaCommand) Name() string        { return "upload-media" }
func (c *UploadMediaCommand) Description() string { return "Upload one or multiple media files" }
func (c *UploadMediaCommand) Group() string       { return "media" }
func (c *UploadMediaCommand) Category() string    { return "🎞️ Media" }
func (c *UploadMediaCommand) UserPermissions() []int64 {
	return []int64{perm.Administrator}
}

func (c *UploadMediaCommand) SlashDefinition() *adapter.SlashCommand {
	opts := []adapter.SlashOption{}
	for i := 1; i <= 10; i++ {
		required := i == 1
		desc := "Upload a media file (image/video/etc)"
		if !required {
			desc = fmt.Sprintf("Optional %s file", ordinal(i))
		}
		opts = append(opts, adapter.SlashOption{
			Type:        adapter.OptionAttachment,
			Name:        fmt.Sprintf("file%d", i),
			Description: desc,
			Required:    required,
		})
	}
	// Optional string goes last
	opts = append(opts, adapter.SlashOption{
		Type:        adapter.OptionString,
		Name:        "category",
		Description: "Tag or category for the uploaded media (e.g. memes, cats)",
		Required:    false,
	})
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options:     opts,
	}
}

func ordinal(i int) string {
	switch i {
	case 2:
		return "2nd"
	case 3:
		return "3rd"
	default:
		return fmt.Sprintf("%dth", i)
	}
}

func (c *UploadMediaCommand) Run(ctx *adapter.SlashInteractionContext) error {
	if err := ctx.DeferEphemeral(); err != nil {
		ctx.AppLog.Error().Err(err).Msg("media_upload_defer_failed")
		return err
	}

	category := "uncategorized"
	var files []adapter.Attachment

	for _, arg := range ctx.Arguments {
		switch arg.Type {
		case adapter.OptionString:
			if arg.Name == "category" {
				category = sanitizeCategory(arg.StringValue())
			}
		case adapter.OptionAttachment:
			if att, ok := ctx.Attachments[arg.StringValue()]; ok {
				files = append(files, att)
			}
		}
	}

	if len(files) == 0 {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: "No files uploaded.",
			Color:       reply.EmbedColor,
		})
	}

	saved := 0
	failed := 0

	for _, file := range files {
		if err := saveUploadedFile(file, ctx.GuildID(), category); err != nil {
			ctx.AppLog.Error().Str("filename", file.Name).Err(err).Msg("media_upload_save_failed")
			failed++
			continue
		}
		saved++
	}

	return ctx.FollowupEphemeral(&adapter.Embed{
		Title: "📥 Media Upload",
		Description: fmt.Sprintf(
			"Saved **%d** file(s) to category `%s` (%d failed)",
			saved, category, failed,
		),
		Color: reply.EmbedColor,
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

func saveUploadedFile(att adapter.Attachment, guildID, category string) error {
	resp, err := http.Get(att.URL)
	if err != nil {
		return fmt.Errorf("media: download attachment: `%v`", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("media: bad response downloading file: `%v`", resp.Status)
	}

	dir := filepath.Join("assets", "media", guildID, category)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("media: create dir: `%v`", err)
	}

	destPath := filepath.Join(dir, att.Name)
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("media: create file: `%v`", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("media: write file: `%v`", err)
	}

	return nil
}
