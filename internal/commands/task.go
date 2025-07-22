package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	cooldownDuration = time.Minute * 60 * 3

	taskCancels     = make(map[string]context.CancelFunc)
	taskCancelMutex = sync.Mutex{}
	tasks           = []Task{}
)

type Task struct {
	Description  string
	DurationMin  int
	RolesAllowed []string
}

func init() {
	Register(&Command{
		Sort:               20,
		Name:               "task",
		Description:        "Assign or manage your personal task, slave.",
		Category:           "🎭 Roleplay",
		DCSlashHandler:     taskSlashHandler,
		DCComponentHandler: taskComponentHandler,
	})

	cfg := config.New()
	var err error
	tasks, err = loadTasks(cfg.TasksPath)
	if err != nil {
		log.Println("Failed to load tasks:", err)
		return
	}
	if len(tasks) == 0 {
		log.Println("No tasks loaded! Aborting task assignment.")
		return
	}

	log.Printf("Loaded %d tasks from %s\n", len(tasks), cfg.TasksPath)
}

func taskSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.InteractionCreate
	userID, guildID := i.Member.User.ID, i.GuildID

	// Cooldown check
	if cooldownUntil, err := ctx.Storage.GetCooldown(guildID, userID); err == nil && time.Now().Before(cooldownUntil) {
		remaining := time.Until(cooldownUntil)
		respondEphemeral(s, i, fmt.Sprintf("Not so fast, darling. You need to wait **%s** before I'm ready to play again with you.", humanDuration(remaining)))
		return
	}

	// Protected user check
	cfg := config.New()
	if slices.Contains(cfg.ProtectedUsers, userID) {
		respond(s, i, "Some lines even a Domme bot won’t cross—especially the one drawn by its creator. No tasks for the one who commands the code. 😈")
		return
	}

	// Cancel previous task (if exists)
	taskCancelMutex.Lock()
	if cancel, exists := taskCancels[userID]; exists {
		cancel()
		delete(taskCancels, userID)
	}
	taskCancelMutex.Unlock()

	// Existing task check
	if existing, _ := ctx.Storage.GetUserTask(guildID, userID); existing != nil && existing.Status == "pending" {
		respondEphemeral(s, i, "You already have a task, darling. Finish one before begging for more.")
		return
	}

	// Role validation
	taskerRoleIDs, err := ctx.Storage.GetTaskRoles(guildID)
	if err != nil || len(taskerRoleIDs) == 0 {
		respondEphemeral(s, i, "No 'tasker' roles configured, darling. I can't just let anyone play with my toys.")
		return
	}

	// Filter tasks based on user roles
	memberRoleNames := getMemberRoleNames(s, guildID, i.Member.Roles)
	filteredTasks := filterTasksByRoles(tasks, memberRoleNames)
	if len(filteredTasks) == 0 {
		respondEphemeral(s, i, "None of the tasks are suitable for someone of your… questionable qualifications.")
		return
	}

	task := filteredTasks[rand.Intn(len(filteredTasks))]
	assignTask(ctx, i, task)
}

func taskComponentHandler(ctx *ComponentContext) {
	s, i := ctx.Session, ctx.InteractionCreate
	userID, guildID := i.Member.User.ID, i.GuildID

	task, err := ctx.Storage.GetUserTask(guildID, userID)
	if err != nil || task == nil {
		respondEphemeral(s, i, "No active task found. Trying to cheat, hmm?")
		return
	}

	if task.UserID != userID {
		respondEphemeral(s, i, "That task doesn’t belong to you. Greedy little fingers, aren't you?")
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
		handleTaskCompletion(ctx, i, task)
	}
}

func assignTask(ctx *SlashContext, i *discordgo.InteractionCreate, task Task) {
	s := ctx.Session
	userID, guildID := i.Member.User.ID, i.GuildID
	now := time.Now()
	expiry := now.Add(time.Duration(task.DurationMin) * time.Minute)
	expiryDelay := time.Duration(task.DurationMin) * time.Minute
	reminderDelay := time.Duration(float64(expiryDelay) * reminderFraction)

	taskMsg := fmt.Sprintf(
		"**New Task**\n<@%s> %s\n\n*You have %s to complete this task so don't disappoint me.*",
		userID, task.Description, humanDuration(time.Until(expiry)))

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: taskMsg,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.Button{Label: "Manage", Style: discordgo.PrimaryButton, CustomID: "task_complete_trigger"},
				}},
			},
		},
	})
	if err != nil {
		log.Println("Failed to send task response:", err)
		return
	}

	msg, err := s.InteractionResponse(i.Interaction)
	if err != nil {
		log.Println("Failed to fetch interaction response:", err)
		return
	}

	entry := storage.UserTask{
		UserID:     userID,
		MessageID:  msg.ID,
		AssignedAt: now,
		ExpiresAt:  expiry,
		Status:     "pending",
	}
	ctx.Storage.SetUserTask(guildID, userID, entry)

	ctxTimer, cancel := context.WithCancel(context.Background())
	taskCancelMutex.Lock()
	taskCancels[userID] = cancel
	taskCancelMutex.Unlock()

	go handleTimers(ctx, ctxTimer, guildID, userID, i.ChannelID, msg.ID, time.Until(expiry), reminderDelay)

	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, i.Member.User.Username, "task")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}

func handleTaskCompletion(ctx *ComponentContext, i *discordgo.InteractionCreate, task *storage.UserTask) {
	s := ctx.Session
	userID, guildID := i.Member.User.ID, i.GuildID
	customID := i.MessageComponentData().CustomID

	var reply string
	switch customID {
	case "task_complete_yes":
		task.Status = "completed"
		reply = "**Task Completed**\n" + fmt.Sprintf(randomLine(completeYesReplies), userID)
	case "task_complete_no":
		task.Status = "failed"
		reply = "**Task Failed**\n" + fmt.Sprintf(randomLine(completeNoReplies), userID)
	case "task_complete_safeword":
		task.Status = "safeword"
		reply = "**Safeword**\n" + fmt.Sprintf(randomLine(completeSafewordReplies), userID)
	}

	ctx.Storage.ClearUserTask(guildID, userID)
	ctx.Storage.SetCooldown(guildID, userID, time.Now().Add(cooldownDuration))

	taskCancelMutex.Lock()
	if cancel, exists := taskCancels[userID]; exists {
		cancel()
		delete(taskCancels, userID)
	}
	taskCancelMutex.Unlock()

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    i.Message.Content,
			Components: []discordgo.MessageComponent{},
		},
	})
	s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
		Content: reply,
	})
}

func handleTimers(ctx *SlashContext, ctxTimer context.Context, guildID, userID, channelID, taskMsgID string, expiryDelay, reminderDelay time.Duration) {
	select {
	case <-time.After(reminderDelay):
		current, _ := ctx.Storage.GetUserTask(guildID, userID)
		if current != nil && current.Status == "pending" {
			ctx.Session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Content: "**Task Reminder**\n" + fmt.Sprintf(randomLine(taskReminders), userID, humanDuration(expiryDelay-reminderDelay)),
				Reference: &discordgo.MessageReference{
					MessageID: taskMsgID, ChannelID: channelID, GuildID: guildID,
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
			ctx.Session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Content: "**Task Expired**\n" + fmt.Sprintf(randomLine(taskFailures), userID),
				Reference: &discordgo.MessageReference{
					MessageID: taskMsgID, ChannelID: channelID, GuildID: guildID,
				},
			})
			ctx.Storage.ClearUserTask(guildID, userID)
			ctx.Storage.SetCooldown(guildID, userID, time.Now().Add(cooldownDuration))
			ctx.Session.ChannelMessageEditComplex(&discordgo.MessageEdit{
				ID: taskMsgID, Channel: channelID, Components: &[]discordgo.MessageComponent{},
			})
		}
	case <-ctxTimer.Done():
		return
	}
}

func getMemberRoleNames(s *discordgo.Session, guildID string, roleIDs []string) map[string]bool {
	names := make(map[string]bool)
	for _, rid := range roleIDs {
		role, err := s.State.Role(guildID, rid)
		if err != nil || role == nil {
			roles, err := s.GuildRoles(guildID)
			if err != nil {
				continue
			}
			for _, r := range roles {
				if r.ID == rid {
					role = r
					break
				}
			}
		}
		if role != nil {
			names[role.Name] = true
		}
	}
	return names
}

func filterTasksByRoles(all []Task, roles map[string]bool) []Task {
	var filtered []Task
	for _, task := range all {
		if len(task.RolesAllowed) == 0 {
			filtered = append(filtered, task)
			continue
		}
		for _, allowed := range task.RolesAllowed {
			if roles[allowed] {
				filtered = append(filtered, task)
				break
			}
		}
	}
	return filtered
}

func completionButtons() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Yes", Style: discordgo.SuccessButton, CustomID: "task_complete_yes"},
			discordgo.Button{Label: "No", Style: discordgo.DangerButton, CustomID: "task_complete_no"},
			discordgo.Button{Label: "Safeword", Style: discordgo.SecondaryButton, CustomID: "task_complete_safeword"},
		}},
	}
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

func randomLine(list []string) string {
	return list[rand.Intn(len(list))]
}

func loadTasks(filename string) ([]Task, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	return tasks, json.Unmarshal(data, &tasks)
}
