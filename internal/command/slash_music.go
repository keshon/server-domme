package command

import (
	"fmt"
	"server-domme/internal/core"
	"server-domme/internal/music/player"

	"github.com/bwmarrin/discordgo"
)

func listenPlayerStatusSlash(session *discordgo.Session, event *discordgo.InteractionCreate, p *player.Player) {
	go func() {
		for signal := range p.PlayerStatus {
			switch signal {
			case player.StatusPlaying:
				track := p.CurrentTrack()
				if track == nil {
					session.FollowupMessageCreate(event.Interaction, false, &discordgo.WebhookParams{
						Content: "⚠️ Failed to get current track",
					})
					return
				}

				var desc string
				if track.Title != "" && track.URL != "" {
					desc = fmt.Sprintf("🎶 [%s](%s)", track.Title, track.URL)
				} else if track.Title != "" {
					desc = "🎶 " + track.Title
				} else if track.URL != "" {
					desc = "🎶 " + track.URL
				} else {
					desc = "🎶 Unknown track"
				}

				session.FollowupMessageCreate(event.Interaction, false, &discordgo.WebhookParams{
					Embeds: []*discordgo.MessageEmbed{{
						Title:       player.StatusPlaying.StringEmoji() + " Now Playing",
						Description: desc,
						Color:       core.EmbedColor,
					}},
				})
				return

			case player.StatusAdded:
				session.FollowupMessageCreate(event.Interaction, false, &discordgo.WebhookParams{
					Embeds: []*discordgo.MessageEmbed{{
						Title:       player.StatusAdded.StringEmoji() + " Track(s) Added",
						Description: "Added to queue",
						Color:       core.EmbedColor,
					}},
				})
				return

			}
		}
	}()
}
