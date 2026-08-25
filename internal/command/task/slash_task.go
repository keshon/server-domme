package task

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	st "github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

var (
	reminderFraction        = 0.9 // 10% before expiry
	defaultCooldownDuration = st.DefaultTaskCooldownDuration

	taskCancels     = make(map[string]context.CancelFunc)
	taskCancelMutex = sync.Mutex{}
	tasks           = []Task{}
)

type Task struct {
	Description  string
	DurationMin  int
	RolesAllowed []string
}

type TaskCommand struct{}

func (c *TaskCommand) Name() string        { return "task" }
func (c *TaskCommand) Description() string { return "Assign yourself a new random task" }
func (c *TaskCommand) Group() string       { return "task" }
func (c *TaskCommand) Category() string    { return "🎭 Roleplay" }
func (c *TaskCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *TaskCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *TaskCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*cmdadapter.SlashInteractionContext)
	if !ok {
		return nil
	}
	return c.runSelfAssign(context)
}

func (c *TaskCommand) runSelfAssign(context *cmdadapter.SlashInteractionContext) error {

	session := context.Session
	event := context.Event
	storage := context.Storage

	guildID := event.GuildID
	member := event.Member
	userID := member.User.ID

	if cooldownUntil, err := storage.GetCooldown(guildID, userID); err == nil && time.Now().Before(cooldownUntil) {
		reply.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("You're on cooldown.\nYou can do this again in %s", humanDuration(time.Until(cooldownUntil))),
		})
		return nil
	}

	if context.Config != nil && slices.Contains(context.Config.ProtectedUsers, userID) {
		return reply.RespondEmbed(session, event, &discordgo.MessageEmbed{
			Description: "You're above this. No tasks for you.",
		})
	}

	taskCancelMutex.Lock()
	if cancel, exists := taskCancels[userID]; exists {
		cancel()
		delete(taskCancels, userID)
	}
	taskCancelMutex.Unlock()

	existing, _ := storage.GetTask(guildID, userID)
	if existing != nil && existing.Status == "pending" {
		reply.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "You already have a task pending.",
		})
		return nil
	}

	taskerRoles, _ := storage.GetTaskRole(guildID)
	if len(taskerRoles) == 0 {
		reply.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "No tasker roles set. Ask an Admin to set them.",
		})
		return nil
	}

	memberRoleNames := getMemberRoleNames(session, guildID, event.Member.Roles)
	tasks, err := loadTasksForGuild(guildID)
	if err != nil {
		reply.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "Failed to load tasks.\nAsk an Admin to set them.",
		})
		context.AppLog.Error().Str("guild_id", guildID).Err(err).Msg("task_list_load_failed")
		return nil
	}

	filtered := filterTasksByRoles(tasks, memberRoleNames)
	if len(filtered) == 0 {
		reply.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "No task suits your... profile.\nAsk an Admin to upload tasks for your gender role and try again.",
		})
		return nil
	}

	task := filtered[rand.Intn(len(filtered))]
	c.assignTask(context.AppLog, session, event, task, storage)

	return nil
}

func loadTasksForGuild(guildID string) ([]Task, error) {
	file := filepath.Join("data", fmt.Sprintf("%s_task.list.json", guildID))
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	return tasks, json.Unmarshal(raw, &tasks)
}

func (c *TaskCommand) assignTask(log zerolog.Logger, session *discordgo.Session, event *discordgo.InteractionCreate, task Task, storage *st.Storage) {
	guildID := event.GuildID
	userID := event.Member.User.ID

	now := time.Now()
	expiry := now.Add(time.Duration(task.DurationMin) * time.Minute)
	expiryDelay := time.Duration(task.DurationMin) * time.Minute
	reminderDelay := time.Duration(float64(expiryDelay) * reminderFraction)

	taskMsg := fmt.Sprintf(
		"**New Task**\n<@%s> %s\n\n*You have %s to complete this task so don't disappoint me.*",
		userID, task.Description, humanDuration(time.Until(expiry)))

	err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
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
		log.Error().Err(err).Msg("task_respond_failed")
		return
	}

	msg, err := session.InteractionResponse(event.Interaction)
	if err != nil {
		log.Error().Err(err).Msg("task_response_fetch_failed")
		return
	}

	entry := st.Task{
		UserID:     userID,
		MessageID:  msg.ID,
		AssignedAt: now,
		ExpiresAt:  expiry,
		Status:     "pending",
	}
	// Bail if the record did not land: the timers below drive off it, and an
	// unrecorded task would leave the holder with a button and no state behind it.
	if err := storage.SetTask(guildID, userID, entry); err != nil {
		log.Error().Str("guild_id", guildID).Str("user_id", userID).Err(err).Msg("task_store_failed")
		return
	}

	ctxTimer, cancel := context.WithCancel(context.Background())
	taskCancelMutex.Lock()
	taskCancels[userID] = cancel
	taskCancelMutex.Unlock()

	go handleTimers(log, session, storage, ctxTimer, guildID, userID, event.ChannelID, msg.ID, time.Until(expiry), reminderDelay)

}

func (c *TaskCommand) Component(ctx *cmdadapter.ComponentInteractionContext) error {
	session := ctx.Session
	event := ctx.Event
	guildID := event.GuildID
	userID := event.Member.User.ID

	task, err := ctx.Storage.GetTask(guildID, userID)
	if err != nil || task == nil {
		reply.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "No active task found. Trying to cheat, hmm?",
		})
		return nil
	}

	if task.UserID != userID {
		reply.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: "That task doesn’t belong to you. Greedy little fingers, aren't you?",
		})
		return nil
	}

	if task.Status != "pending" {
		if err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		}); err != nil {
			ctx.AppLog.Error().Str("custom_id", event.MessageComponentData().CustomID).Err(err).Msg("task_defer_update_failed")
		}
		return nil
	}

	switch event.MessageComponentData().CustomID {
	case "task_complete_trigger":
		if err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content: event.Message.Content,
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{Components: []discordgo.MessageComponent{
						discordgo.Button{Label: "Yes", Style: discordgo.SuccessButton, CustomID: "task_complete_yes"},
						discordgo.Button{Label: "No", Style: discordgo.DangerButton, CustomID: "task_complete_no"},
						discordgo.Button{Label: "Safeword", Style: discordgo.SecondaryButton, CustomID: "task_complete_safeword"},
					}},
				},
			},
		}); err != nil {
			ctx.AppLog.Error().Err(err).Msg("task_completion_prompt_failed")
		}
	case "task_complete_yes", "task_complete_no", "task_complete_safeword":
		c.handleTaskCompletion(ctx, event, task)
	}

	return nil
}

func (c *TaskCommand) handleTaskCompletion(ctx *cmdadapter.ComponentInteractionContext, event *discordgo.InteractionCreate, task *st.Task) {
	session := ctx.Session
	userID, guildID := event.Member.User.ID, event.GuildID
	customID := event.MessageComponentData().CustomID

	var msg string
	switch customID {
	case "task_complete_yes":
		task.Status = "completed"
		msg = "**Task Completed**\n" + fmt.Sprintf(randomLine(completeYesReplies), userID)
	case "task_complete_no":
		task.Status = "failed"
		msg = "**Task Failed**\n" + fmt.Sprintf(randomLine(completeNoReplies), userID)
	case "task_complete_safeword":
		task.Status = "safeword"
		msg = "**Safeword**\n" + fmt.Sprintf(randomLine(completeSafewordReplies), userID)
	}

	if err := ctx.Storage.ClearTask(guildID, userID); err != nil {
		ctx.AppLog.Error().Str("guild_id", guildID).Str("user_id", userID).Err(err).Msg("task_clear_failed")
	}
	if err := ctx.Storage.SetCooldown(guildID, userID, time.Now().Add(cooldownForGuild(ctx.Storage, guildID))); err != nil {
		ctx.AppLog.Error().Str("guild_id", guildID).Str("user_id", userID).Err(err).Msg("task_cooldown_set_failed")
	}

	taskCancelMutex.Lock()
	if cancel, exists := taskCancels[userID]; exists {
		cancel()
		delete(taskCancels, userID)
	}
	taskCancelMutex.Unlock()

	if err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    event.Message.Content,
			Components: []discordgo.MessageComponent{},
		},
	}); err != nil {
		ctx.AppLog.Error().Str("custom_id", customID).Err(err).Msg("task_completion_ack_failed")
	}
	if _, err := session.FollowupMessageCreate(event.Interaction, false, &discordgo.WebhookParams{
		Content: msg,
	}); err != nil {
		ctx.AppLog.Error().Str("custom_id", customID).Err(err).Msg("task_completion_followup_failed")
	}
}

// InitFromConfig loads the default task list from cfg.TasksPath. Call from main after loading config.
func InitFromConfig(cfg *config.Config, log zerolog.Logger) error {
	if cfg == nil {
		return nil
	}
	loaded, err := loadTasks(cfg.TasksPath)
	if err != nil {
		return err
	}
	tasks = loaded
	if len(tasks) == 0 {
		log.Warn().Str("path", cfg.TasksPath).Msg("task_list_empty")
		return nil
	}
	log.Info().Int("count", len(tasks)).Str("path", cfg.TasksPath).Msg("task_list_loaded")
	return nil
}

func handleTimers(log zerolog.Logger, session *discordgo.Session, storage *st.Storage, ctxTimer context.Context, guildID, userID, channelID, taskMsgID string, expiryDelay, reminderDelay time.Duration) {
	select {
	case <-time.After(reminderDelay):
		current, _ := storage.GetTask(guildID, userID)
		if current != nil && current.Status == "pending" {
			if _, err := session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Content: "**Task Reminder**\n" + fmt.Sprintf(randomLine(taskReminders), userID, humanDuration(expiryDelay-reminderDelay)),
				Reference: &discordgo.MessageReference{
					MessageID: taskMsgID, ChannelID: channelID, GuildID: guildID,
				},
			}); err != nil {
				log.Warn().Str("channel_id", channelID).Err(err).Msg("task_reminder_failed")
			}
		}
	case <-ctxTimer.Done():
		return
	}

	select {
	case <-time.After(expiryDelay - reminderDelay):
		current, _ := storage.GetTask(guildID, userID)
		if current != nil && current.Status == "pending" {
			if _, err := session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Content: "**Task Expired**\n" + fmt.Sprintf(randomLine(taskFailures), userID),
				Reference: &discordgo.MessageReference{
					MessageID: taskMsgID, ChannelID: channelID, GuildID: guildID,
				},
			}); err != nil {
				log.Warn().Str("channel_id", channelID).Err(err).Msg("task_expiry_notice_failed")
			}
			if err := storage.ClearTask(guildID, userID); err != nil {
				log.Error().Str("guild_id", guildID).Str("user_id", userID).Err(err).Msg("task_clear_failed")
			}
			if err := storage.SetCooldown(guildID, userID, time.Now().Add(cooldownForGuild(storage, guildID))); err != nil {
				log.Error().Str("guild_id", guildID).Str("user_id", userID).Err(err).Msg("task_cooldown_set_failed")
			}
			// Strip the Manage button: the task is over, and leaving it live would
			// let the holder answer a prompt with no record behind it.
			if _, err := session.ChannelMessageEditComplex(&discordgo.MessageEdit{
				ID: taskMsgID, Channel: channelID, Components: &[]discordgo.MessageComponent{},
			}); err != nil {
				log.Warn().Str("channel_id", channelID).Err(err).Msg("task_button_strip_failed")
			}
		}
	case <-ctxTimer.Done():
		return
	}
}

func loadTasks(file string) ([]Task, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var list []Task
	return list, json.Unmarshal(raw, &list)
}

func cooldownForGuild(storage *st.Storage, guildID string) time.Duration {
	duration, err := storage.GetTaskCooldownDuration(guildID)
	if err != nil {
		return defaultCooldownDuration
	}
	return duration
}

func getMemberRoleNames(session *discordgo.Session, guildID string, roleIDs []string) map[string]bool {
	names := make(map[string]bool)
	for _, rid := range roleIDs {
		role, err := session.State.Role(guildID, rid)
		if err != nil || role == nil {
			allRoles, _ := session.GuildRoles(guildID)
			for _, r := range allRoles {
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
	var out []Task
	for _, task := range all {
		if len(task.RolesAllowed) == 0 {
			out = append(out, task)
			continue
		}
		for _, r := range task.RolesAllowed {
			if roles[r] {
				out = append(out, task)
				break
			}
		}
	}
	return out
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
