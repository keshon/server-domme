// /internal/commands/about.go
package commands

import (
	"fmt"
	"log"
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
		Sort:           920,                                               // sorting weight
		Name:           "about",                                           // command name
		Description:    "Discover the origin of your merciless mistress.", // command description
		Category:       "🕯️ Lore & Insight",                               // command category
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

func aboutSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.InteractionCreate

	emb, file, err := buildAboutMessage()
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Failed to build about message: ```%v```", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	resp := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{emb},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	}

	if file != nil {
		resp.Data.Files = []*discordgo.File{file}
	}

	s.InteractionRespond(i.Interaction, resp)

	guildID := i.GuildID
	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "about")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}
