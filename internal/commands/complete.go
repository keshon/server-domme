package commands

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:               101,
		Name:               "complete",
		Description:        "Respond to your active task with Yes, No, or Safeword",
		Category:           "Tasks",
		DCSlashHandler:     completeSlashHandler,
		DCComponentHandler: completeComponentHandler,
	})
}

func completeSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.Interaction

	userID := i.Member.User.ID
	guildID := i.GuildID
	storage := ctx.Storage

	task, err := storage.GetUserTask(guildID, userID)
	if err != nil || task == nil || task.Status != "pending" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No active task found. Are you lost, pet?",
				Flags:   1 << 6,
			},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("<@%s> Did you complete your task, slut? Be honest… or not.", userID),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{Label: "Yes", Style: discordgo.SuccessButton, CustomID: "complete_yes"}, // complete prefix MUST be equal to the command name
						discordgo.Button{Label: "No", Style: discordgo.DangerButton, CustomID: "complete_no"},
						discordgo.Button{Label: "Safeword", Style: discordgo.SecondaryButton, CustomID: "complete_safeword"},
					},
				},
			},
		},
	})
}

func completeComponentHandler(ctx *ComponentContext) {
	s, i := ctx.Session, ctx.Interaction
	userID := i.Member.User.ID
	guildID := i.GuildID
	storage := ctx.Storage

	task, err := storage.GetUserTask(guildID, userID)
	if err != nil || task == nil || task.Status != "pending" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No active task to respond to. Trying to fake your way out, hmm?",
				Flags:   1 << 6,
			},
		})
		return
	}

	var reply string
	switch i.MessageComponentData().CustomID {
	case "complete_yes":
		task.Status = "completed"
		reply = fmt.Sprintf("💎 <@%s> finally did something right. 3000 coins awarded. You may breathe.", userID)
	case "complete_no":
		task.Status = "failed"
		reply = fmt.Sprintf("🙄 <@%s> admits failure. Predictable. No reward, just shame.", userID)
	case "complete_safeword":
		task.Status = "safeword"
		reply = fmt.Sprintf("⚠️ <@%s> used the safeword. Task canceled. Consent respected. This time.", userID)
	default:
		reply = "Unrecognized response. Are those fingers too clumsy?"
	}

	storage.ClearUserTask(guildID, userID)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: reply,
		},
	})
}
