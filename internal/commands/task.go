package commands

import (
	"fmt"
	"math/rand"
	"server-domme/internal/storage"
	"time"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           100,
		Name:           "task",
		Description:    "Assign a random task to the user",
		Category:       "Tasks",
		DCSlashHandler: taskSlashHandler,
	})
}

var tasks = []string{
	"💋 Time to dance! Find a classic Backstreet Boys song and show me your best boy band moves.",
	"🍌 Eat a banana seductively and post the aftermath.",
	"📸 Take a selfie with your most bratty expression. Don’t hold back.",
}

func taskSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.Interaction

	if i.Member == nil || i.Member.User == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "I need to know who you are, darling. No cloak of invisibility allowed.",
				Flags:   1 << 6,
			},
		})
		return
	}

	userID := i.Member.User.ID
	guildID := i.GuildID

	if existingTask, _ := ctx.Storage.GetUserTask(guildID, userID); existingTask != nil && existingTask.Status == "pending" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You already have a task, darling. Finish one before begging for more.",
				Flags:   1 << 6,
			},
		})
		return
	}

	task := tasks[rand.Intn(len(tasks))]
	taskText := fmt.Sprintf("<@%s> %s\n\nCompleting this task will earn you 3000 coins. You have 1 hour to submit proof. Don’t disappoint me.", userID, task)

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: taskText,
		},
	})
	if err != nil {
		fmt.Println("Interaction respond error:", err)
		return
	}

	now := time.Now()
	taskEntry := storage.UserTask{
		UserID:     userID,
		TaskText:   task,
		AssignedAt: now,
		ExpiresAt:  now.Add(1 * time.Hour),
		Status:     "pending",
	}

	err = ctx.Storage.SetUserTask(guildID, userID, taskEntry)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Couldn't save your task. The universe must hate you today.",
				Flags:   1 << 6,
			},
		})
		return
	}

	go func() {
		time.Sleep(1 * time.Hour)
		current, err := ctx.Storage.GetUserTask(guildID, userID)
		if err == nil && current != nil && current.Status == "pending" {
			s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("⏰ <@%s> failed to complete the task in time. Naughty little disappointment.", userID))
			ctx.Storage.ClearUserTask(guildID, userID)
		}
	}()
}
