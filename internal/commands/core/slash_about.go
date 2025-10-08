package core

import (
	"log"
	"os"
	"path/filepath"
	"server-domme/internal/core"
	"server-domme/internal/version"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type AboutCommand struct{}

func (c *AboutCommand) Name() string        { return "about" }
func (c *AboutCommand) Description() string { return "Discover the origin of this bot" }
func (c *AboutCommand) Aliases() []string   { return []string{} }
func (c *AboutCommand) Group() string       { return "core" }
func (c *AboutCommand) Category() string    { return "🕯️ Information" }
func (c *AboutCommand) RequireAdmin() bool  { return false }
func (c *AboutCommand) RequireDev() bool    { return false }

func (c *AboutCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *AboutCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session, event, storage := context.Session, context.Event, context.Storage
	member := event.Member

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
		{
			Name:  "Developed by Señor Mega",
			Value: "[LinkedIn](https://www.linkedin.com/in/keshon), [GitHub](https://github.com/keshon), [Homepage](https://keshon.ru)",
		},
		{
			Name:  "Repository",
			Value: "https://github.com/keshon/server-domme",
		},
		{
			Name:  "Release",
			Value: buildDate + " (Go " + goVer + ")",
		},
	}

	// Create embed
	embed := &discordgo.MessageEmbed{
		Title:       "ℹ️ About " + version.AppName,
		Description: version.AppDescription,
		Color:       core.EmbedColor,
		Fields:      fields,
	}

	// Try attaching banner if exists
	imagePath := "./assets/about-banner.webp"
	if f, err := os.Open(imagePath); err == nil {
		defer f.Close()
		imageName := filepath.Base(imagePath)
		embed.Image = &discordgo.MessageEmbedImage{URL: "attachment://" + imageName}
		return core.RespondEmbedEphemeralWithFile(session, event, embed, f, imageName)
	}

	// Just embed if no banner
	if err := core.RespondEmbedEphemeral(session, event, embed); err != nil {
		return err
	}

	// Log usage
	if err := core.LogCommand(session, storage, event.GuildID, event.ChannelID, member.User.ID, member.User.Username, c.Name()); err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&AboutCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}
