package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"server-domme/internal/config"
	"server-domme/internal/storage"
	"slices"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	reminderFraction = 0.9 // 10% before expiry
	taskCancels      = make(map[string]context.CancelFunc)
	taskCancelMutex  = sync.Mutex{}
	tasks            = []Task{}
)

type Task struct {
	Description string
	DurationMin int
}

func loadTasks(filename string) ([]Task, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	err = json.Unmarshal(data, &tasks)
	return tasks, err
}

func init() {
	Register(&Command{
		Sort:               100,
		Name:               "task",
		Description:        "Assigns and manages your task",
		Category:           "Tasks",
		DCSlashHandler:     taskSlashHandler,
		DCComponentHandler: taskComponentHandler,
	})

	cfg := config.New()
	var err error
	tasks, err = loadTasks(cfg.TasksPath)

	if err != nil {
		fmt.Println("Failed to load tasks:", err)
		return
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks loaded! Aborting task assignment.")
		return
	}

	fmt.Printf("Loaded %d tasks from %s\n", len(tasks), cfg.TasksPath)
}

func taskSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.Interaction
	userID := i.Member.User.ID
	guildID := i.GuildID

	cfg := config.New()
	if slices.Contains(cfg.ProtectedUsers, userID) {
		_ = ctx.Session.InteractionRespond(ctx.Interaction.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Some lines even a Domme bot won’t cross—especially the one drawn by its creator. No tasks for the one who commands the code. The puppet never pulls its own strings. 😈",
			},
		})
		return
	}

	taskCancelMutex.Lock()
	if cancel, exists := taskCancels[userID]; exists {
		cancel()
		delete(taskCancels, userID)
	}
	taskCancelMutex.Unlock()

	if existing, _ := ctx.Storage.GetUserTask(guildID, userID); existing != nil && existing.Status == "pending" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You already have a task, darling. Finish one before begging for more.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	task := tasks[rand.Intn(len(tasks))]
	now := time.Now()

	expiryDelay := time.Duration(task.DurationMin) * time.Minute
	reminderDelay := time.Duration(float64(expiryDelay) * reminderFraction)

	expiry := now.Add(expiryDelay)
	expiryText := humanDuration(expiryDelay)

	taskMsg := fmt.Sprintf(
		"**New Task**\n<@%s> %s\n\n*You have %s to complete this task so don't disappoint me.\nWhen you're done (or if you’re too weak to go on), press the button below.*",
		userID, task.Description, expiryText)

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: taskMsg,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{Label: "Manage", Style: discordgo.PrimaryButton, CustomID: "task_complete_trigger"},
					},
				},
			},
		},
	})
	if err != nil {
		fmt.Println("Failed to send task response:", err)
		return
	}

	msg, err := s.InteractionResponse(i.Interaction)
	if err != nil {
		fmt.Println("Failed to fetch interaction response:", err)
		return
	}

	taskEntry := storage.UserTask{
		UserID:     userID,
		MessageID:  msg.ID,
		TaskText:   task.Description,
		AssignedAt: now,
		ExpiresAt:  expiry,
		Status:     "pending",
	}
	ctx.Storage.SetUserTask(guildID, userID, taskEntry)

	ctxTimer, cancel := context.WithCancel(context.Background())

	taskCancelMutex.Lock()
	taskCancels[userID] = cancel
	taskCancelMutex.Unlock()

	go handleTimers(ctx, ctxTimer, guildID, userID, i.ChannelID, msg.ID, expiryDelay, reminderDelay)

}

func handleTimers(ctx *SlashContext, ctxTimer context.Context, guildID, userID, channelID, taskMsgID string, expiryDelay, reminderDelay time.Duration) {
	select {
	case <-time.After(reminderDelay):
		current, _ := ctx.Storage.GetUserTask(guildID, userID)
		if current != nil && current.Status == "pending" {
			reminder := fmt.Sprintf(randomLine(taskReminders), userID, humanDuration(expiryDelay-reminderDelay))
			prefixedReminder := "**Task Reminder**\n" + reminder
			ctx.Session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Content: prefixedReminder,
				Reference: &discordgo.MessageReference{
					MessageID: taskMsgID,
					ChannelID: channelID,
					GuildID:   guildID,
				},
			})
		}
	case <-ctxTimer.Done():
		return
	}

	select {
	case <-time.After(expiryDelay - reminderDelay):
		current, _ := ctx.Storage.GetUserTask(guildID, userID)
		if current != nil && current.Status == "pending" {
			failMsg := fmt.Sprintf(randomLine(taskFailures), userID)
			prefixedFailMsg := "**Task Expired**\n" + failMsg
			ctx.Session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Content: prefixedFailMsg,
				Reference: &discordgo.MessageReference{
					MessageID: taskMsgID,
					ChannelID: channelID,
					GuildID:   guildID,
				},
			})
			ctx.Storage.ClearUserTask(guildID, userID)
			empty := []discordgo.MessageComponent{}
			ctx.Session.ChannelMessageEditComplex(&discordgo.MessageEdit{
				ID:         taskMsgID,
				Channel:    channelID,
				Components: &empty,
			})
		}
	case <-ctxTimer.Done():
		return
	}
}

func taskComponentHandler(ctx *ComponentContext) {
	s, i := ctx.Session, ctx.Interaction
	userID := i.Member.User.ID
	guildID := i.GuildID

	task, err := ctx.Storage.GetUserTask(guildID, userID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Something went wrong fetching your task. Probably your fault (or this bot is broken).",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if task == nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No active task found. Trying to cheat, hmm?",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if task.UserID != userID {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "That task doesn’t belong to you. Greedy little fingers, aren't you?",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if task.Status != "pending" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		})
		return
	}

	switch i.MessageComponentData().CustomID {
	case "task_complete_trigger":
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    i.Message.Content,
				Components: completionButtons(),
			},
		})

	case "task_complete_yes", "task_complete_no", "task_complete_safeword":
		var reply string
		switch i.MessageComponentData().CustomID {
		case "task_complete_yes":
			task.Status = "completed"
			reply = fmt.Sprintf(randomLine(completeYesReplies), userID)
			reply = "**Task Completed**\n" + reply

		case "task_complete_no":
			task.Status = "failed"
			reply = fmt.Sprintf(randomLine(completeNoReplies), userID)
			reply = "**Task Failed**\n" + reply

		case "task_complete_safeword":
			task.Status = "safeword"
			reply = fmt.Sprintf(randomLine(completeSafewordReplies), userID)
			reply = "**Safeword**\n" + reply
		}

		taskCancelMutex.Lock()
		if cancel, exists := taskCancels[userID]; exists {
			cancel()
			delete(taskCancels, userID)
		}
		taskCancelMutex.Unlock()

		ctx.Storage.ClearUserTask(guildID, userID)

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    i.Message.Content,              // keep original message
				Components: []discordgo.MessageComponent{}, // remove buttons
			},
		})

		s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
			Content: reply,
		})
	}
}

func completionButtons() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Yes", Style: discordgo.SuccessButton, CustomID: "task_complete_yes"},
				discordgo.Button{Label: "No", Style: discordgo.DangerButton, CustomID: "task_complete_no"},
				discordgo.Button{Label: "Safeword", Style: discordgo.SecondaryButton, CustomID: "task_complete_safeword"},
			},
		},
	}
}

func randomLine(list []string) string {
	return list[rand.Intn(len(list))]
}

func humanDuration(d time.Duration) string {
	if d.Hours() >= 1 {
		return fmt.Sprintf("%d hour%s", int(d.Hours()), pluralize(int(d.Hours())))
	}
	if d.Minutes() >= 1 {
		return fmt.Sprintf("%d minute%s", int(d.Minutes()), pluralize(int(d.Minutes())))
	}
	return fmt.Sprintf("%d second%s", int(d.Seconds()), pluralize(int(d.Seconds())))
}

func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
