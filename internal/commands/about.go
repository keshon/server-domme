// /internal/commands/about.go
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"server-domme/internal/version"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	embed "github.com/clinet/discordgo-embed"
)

func init() {
	Register(&Command{
		Sort:        500,                         // sorting weight
		Name:        "about",                     // command name
		Description: "Shows info about the bot.", // command description
		Category:    "Information",               // command category

		DCSlashHandler: aboutSlashHandler,
	})
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
			SetColor(embedColor).
			SetDescription(fmt.Sprintf("ℹ️ About\n\n**%s** — %s", version.AppName, version.AppDescription))
		for title, value := range infoFields {
			embedMsg = embedMsg.AddField(title, value)
		}
		return embedMsg.MessageEmbed, nil, nil
	}

	embedMsg := embed.NewEmbed().
		SetColor(embedColor).
		SetDescription(fmt.Sprintf("ℹ️ About\n\n**%s** — %s", version.AppName, version.AppDescription))
	for title, value := range infoFields {
		embedMsg = embedMsg.AddField(title, value)
	}
	embedMsg = embedMsg.SetImage("attachment://" + imageName)

	return embedMsg.MessageEmbed, &discordgo.File{
		Name:   imageName,
		Reader: imageFile,
	}, nil
}

func aboutSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.Interaction

	emb, file, err := buildAboutMessage()
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Failed to build about message: ```%v```", err),
			},
		})
		return
	}

	resp := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{emb},
		},
	}

	if file != nil {
		resp.Data.Files = []*discordgo.File{file}
	}

	s.InteractionRespond(i.Interaction, resp)
}
