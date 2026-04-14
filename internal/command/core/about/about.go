package about

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"server-domme/internal/command"
	"server-domme/internal/discord"
	"server-domme/internal/version"

	"github.com/bwmarrin/discordgo"
)

type AboutCommand struct{}

func (c *AboutCommand) Name() string        { return "about" }
func (c *AboutCommand) Description() string { return "Discover the origin of this bot" }
func (c *AboutCommand) Group() string       { return "core" }
func (c *AboutCommand) Category() string    { return "🕯️ Information" }
func (c *AboutCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *AboutCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *AboutCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*command.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event

	// Format build date
	buildDate := "unknown"
	if version.BuildDate != "" {
		if t, err := time.Parse(time.RFC3339, version.BuildDate); err == nil {
			buildDate = t.Format("2006-01-02")
		} else {
			buildDate = "invalid date"
		}
	}

	// Get Go version
	goVer := strings.TrimPrefix(version.GoVersion, "go")
	if goVer == "" {
		goVer = "unknown"
	}

	// Info fields for embed
	fields := []*discordgo.MessageEmbedField{
		{Name: "App", Value: version.AppName, Inline: true},
		{Name: "Build Date", Value: buildDate, Inline: true},
		{Name: "Go", Value: goVer, Inline: true},
	}

	// Optional license file snippet
	licensePath := filepath.Join(".", "LICENSE")
	if b, err := os.ReadFile(licensePath); err == nil {
		line := strings.Split(string(b), "\n")
		if len(line) > 0 && strings.TrimSpace(line[0]) != "" {
			fields = append(fields, &discordgo.MessageEmbedField{Name: "License", Value: strings.TrimSpace(line[0]), Inline: false})
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:  "About",
		Color:  discord.EmbedColor,
		Fields: fields,
	}

	return discord.RespondEmbedEphemeral(session, event, embed)
}

