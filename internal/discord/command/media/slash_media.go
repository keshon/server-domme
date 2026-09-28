package media

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/keshon/server-domme/internal/discord/adapter"
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

func (c *RandomMediaCommand) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionString,
				Name:        "category",
				Description: "Optional category to pull from (if omitted, uses default or random)",
				Required:    false,
			},
		},
	}
}

func (c *RandomMediaCommand) Run(ctx *adapter.SlashInteractionContext) error {
	category := ctx.StringOption("category")

	if category == "" && ctx.Storage != nil {
		if defCat, err := ctx.Storage.GetMediaDefault(ctx.GuildID()); err == nil && defCat != "" {
			category = defCat
			ctx.AppLog.Debug().Str("category", defCat).Str("guild_id", ctx.GuildID()).Msg("media_default_category_used")
		}
	}

	// ACK immediately to avoid "application did not respond" on slow
	// disks/large media sets.
	if err := ctx.Defer(); err != nil {
		ctx.AppLog.Warn().Err(err).Msg("media_ack_failed")
	}

	file, err := pickRandomFile(mediaPath(ctx.GuildID(), category))
	if err != nil {
		return ctx.Followup(&adapter.Embed{
			Description: fmt.Sprintf("No media found in `%s`: `%v`", categoryOrDefault(category), err),
		})
	}

	return sendMediaFile(ctx.AppLog, ctx, file, category, displayName(ctx))
}

func (c *RandomMediaCommand) Component(ctx *adapter.ComponentInteractionContext) error {
	ctx.AppLog.Debug().Str("custom_id", ctx.ComponentID).Msg("media_component_received")

	category := ""
	if parts := strings.SplitN(ctx.ComponentID, "|", 2); len(parts) == 2 {
		category = parts[1]
	}

	if category == "" && ctx.Storage != nil {
		if defCat, err := ctx.Storage.GetMediaDefault(ctx.GuildID()); err == nil && defCat != "" {
			category = defCat
			ctx.AppLog.Debug().Str("category", defCat).Str("guild_id", ctx.GuildID()).Msg("media_default_category_used")
		}
	}

	if err := ctx.Defer(); err != nil {
		ctx.AppLog.Error().Err(err).Msg("media_ack_failed")
		return err
	}

	file, err := pickRandomFile(mediaPath(ctx.GuildID(), category))
	if err != nil {
		if ferr := ctx.Followup(&adapter.Embed{
			Description: fmt.Sprintf("No media found in `%s`: `%v`", categoryOrDefault(category), err),
		}); ferr != nil {
			ctx.AppLog.Error().Str("category", category).Err(ferr).Msg("media_followup_failed")
		}
		return nil
	}

	username := ctx.Invoker.DisplayName
	if username == "" {
		username = ctx.UserID()
	}
	if err := sendMediaFile(ctx.AppLog, ctx, file, category, username); err != nil {
		ctx.AppLog.Error().Str("category", category).Err(err).Msg("media_send_failed")
	}
	return nil
}

type mediaFollowup interface {
	FollowupWith(adapter.Reply) error
}

func mediaPath(guildID, category string) string {
	if category != "" {
		return filepath.Join("assets", "media", guildID, category)
	}
	return filepath.Join("assets", "media", guildID)
}

// sendMediaFile posts the file with a Next button. ctx is whichever
// interaction kind asked: both answer followups the same way.
func sendMediaFile(log zerolog.Logger, ctx mediaFollowup, file, category, username string) error {
	f, err := os.Open(file)
	if err != nil {
		_ = ctx.FollowupWith(adapter.Reply{
			Text: fmt.Sprintf("Failed to open media: `%v`", err),
		})
		return err
	}
	defer f.Close()

	return ctx.FollowupWith(adapter.Reply{
		Text:     fmt.Sprintf("`#%s`\n-# Requested by **%s**", categoryOrDefault(category), username),
		File:     f,
		FileName: filepath.Base(file),
		Buttons: []adapter.ActionRow{{
			Buttons: []adapter.Button{
				{Label: "Next", Style: adapter.SecondaryButton, CustomID: fmt.Sprintf("media_next_trigger|%s", category)},
			},
		}},
	})
}

func displayName(ctx *adapter.SlashInteractionContext) string {
	if ctx.Invoker.DisplayName != "" {
		return ctx.Invoker.DisplayName
	}
	return ctx.UserID()
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
		return "", fmt.Errorf("media: no files found")
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
