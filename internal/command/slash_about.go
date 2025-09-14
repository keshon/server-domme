// / internal/command/slash_about.go
package command

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"server-domme/internal/core"
	"server-domme/internal/version"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	embed "github.com/clinet/discordgo-embed"
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
	slash, ok := ctx.(*core.SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	guildID := event.GuildID
	member := event.Member

	embedMsg, file, err := buildAboutMessage()
	if err != nil {
		core.RespondEphemeral(session, event, fmt.Sprintf("Failed to build about message: ```%v```", err))
		return nil
	}

	resp := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embedMsg},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	}
	if file != nil {
		resp.Data.Files = []*discordgo.File{file}
	}

	session.InteractionRespond(event.Interaction, resp)

	err = core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func buildAboutMessage() (*discordgo.MessageEmbed, *discordgo.File, error) {
	buildDate := "unknown"
	if version.BuildDate != "" {
		if t, err := time.Parse(time.RFC3339, version.BuildDate); err == nil {
			buildDate = t.Format("2006-01-02")
		} else {
			buildDate = "invalid date"
		}
	}

	goVer := "unknown"
	if version.GoVersion != "" {
		goVer = strings.TrimPrefix(version.GoVersion, "go")
	}

	infoFields := map[string]string{
		"Developed by Innokentiy Sokolov": "[LinkedIn](https://www.linkedin.com/in/keshon), [GitHub](https://github.com/keshon), [Homepage](https://keshon.ru)",
		"Repository":                      "https://github.com/keshon/server-domme",
		"Release":                         fmt.Sprintf("%s (Go %s)", buildDate, goVer),
	}

	imagePath := "./assets/about-banner.webp"
	imageName := filepath.Base(imagePath)
	imageFile, err := os.Open(imagePath)
	if err != nil {
		embedMsg := embed.NewEmbed().
			SetColor(core.EmbedColor).
			SetDescription(fmt.Sprintf("ℹ️ About\n\n**%s** — %s", version.AppName, version.AppDescription))
		for title, value := range infoFields {
			embedMsg = embedMsg.AddField(title, value)
		}
		return embedMsg.MessageEmbed, nil, nil
	}

	embedMsg := embed.NewEmbed().
		SetColor(core.EmbedColor).
		SetDescription(fmt.Sprintf("ℹ️ **About %s**\n\n%s", version.AppName, version.AppDescription))
	for title, value := range infoFields {
		embedMsg = embedMsg.AddField(title, value)
	}
	embedMsg = embedMsg.SetImage("attachment://" + imageName)

	return embedMsg.MessageEmbed, &discordgo.File{
		Name:   imageName,
		Reader: imageFile,
	}, nil
}

func init() {
	core.RegisterCommand(
		core.WithGroupAccessCheck()(
			core.WithGuildOnly(
				&AboutCommand{},
			),
		),
	)
}
