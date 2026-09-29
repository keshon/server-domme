package task

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/llm"
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

func (c *TaskCommand) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionString,
				Name:        "request",
				Description: "Describe the task you want in plain words",
			},
			{
				Type:        adapter.OptionBoolean,
				Name:        "variant",
				Description: "Rephrase the picked task with AI polish",
			},
		},
	}
}

func (c *TaskCommand) Run(ctx *adapter.SlashInteractionContext) error {
	return c.runSelfAssign(ctx)
}

func (c *TaskCommand) runSelfAssign(ctx *adapter.SlashInteractionContext) error {
	store := ctx.Storage
	guildID := ctx.GuildID()
	userID := ctx.UserID()

	if cooldownUntil, err := store.GetCooldown(guildID, userID); err == nil && time.Now().Before(cooldownUntil) {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("You're on cooldown.\nYou can do this again in %s.", humanDuration(time.Until(cooldownUntil))),
			Color:       reply.EmbedColor,
		})
		return nil
	}

	if ctx.Config != nil && slices.Contains(ctx.Config.ProtectedUsers, userID) {
		return ctx.RespondWith(adapter.Reply{
			Text: "You're above this. No tasks for you.",
		})
	}

	taskCancelMutex.Lock()
	if cancel, exists := taskCancels[userID]; exists {
		cancel()
		delete(taskCancels, userID)
	}
	taskCancelMutex.Unlock()

	existing, _ := store.GetTask(guildID, userID)
	if existing != nil && existing.Status == st.TaskStatusPending {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: "You already have a task pending.",
			Color:       reply.EmbedColor,
		})
		return nil
	}

	taskerRoles, _ := store.GetTaskRole(guildID)
	if len(taskerRoles) == 0 {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: "No tasker roles set. Ask an Admin to set them.",
			Color:       reply.EmbedColor,
		})
		return nil
	}

	roleNames, err := memberRoleNames(ctx.API, guildID, ctx.Invoker.Roles)
	if err != nil {
		ctx.AppLog.Warn().Str("guild_id", guildID).Err(err).Msg("task_roles_resolve_failed")
	}
	tasks, err := loadTasksForGuild(guildID)
	if err != nil {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: "Failed to load tasks.\nAsk an Admin to set them.",
			Color:       reply.EmbedColor,
		})
		ctx.AppLog.Error().Str("guild_id", guildID).Err(err).Msg("task_list_load_failed")
		return nil
	}

	filtered := filterTasksByRoles(tasks, roleNames)
	if len(filtered) == 0 {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: "No task suits your... profile.\nAsk an Admin to upload tasks for your gender role and try again.",
			Color:       reply.EmbedColor,
		})
		return nil
	}

	task := filtered[rand.Intn(len(filtered))]
	if wish := strings.TrimSpace(ctx.StringOption("request")); wish != "" && llm.Ready(ctx.Config) {
		if spec := ParseRequestSpec(context.Background(), llm.ProviderForConfig(ctx.Config), wish); len(spec.Keywords) > 0 || spec.DurationMin > 0 {
			if idx := PickByScore(ScoreTasks(filtered, spec)); idx >= 0 {
				task = filtered[idx]
			}
		}
	}
	if opt, ok := ctx.Option("variant"); ok && opt.BoolValue() && llm.Ready(ctx.Config) {
		task.Description = RephraseTask(context.Background(), llm.ProviderForConfig(ctx.Config), task.Description)
	}
	c.assignTask(ctx, task)

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

func (c *TaskCommand) assignTask(ctx *adapter.SlashInteractionContext, task Task) {
	log := ctx.AppLog
	store := ctx.Storage
	guildID := ctx.GuildID()
	userID := ctx.UserID()

	now := time.Now()
	expiry := now.Add(time.Duration(task.DurationMin) * time.Minute)
	expiryDelay := time.Duration(task.DurationMin) * time.Minute
	reminderDelay := time.Duration(float64(expiryDelay) * reminderFraction)

	taskMsg := fmt.Sprintf(
		"**New Task**\n<@%s> %s\n\n*You have %s to complete this task so don't disappoint me.*",
		userID, task.Description, humanDuration(time.Until(expiry)))

	// Deferred first, same as ask: the answer edits the placeholder, and an
	// unacknowledged interaction has no webhook to edit yet.
	if err := ctx.Defer(); err != nil {
		log.Error().Err(err).Msg("task_ack_failed")
		return
	}

	_, msgID, err := ctx.AnswerTextMessageWithButtons(taskMsg, []adapter.ActionRow{{
		Buttons: []adapter.Button{
			{Label: "Manage", Style: adapter.PrimaryButton, CustomID: "task_complete_trigger"},
		},
	}})
	if err != nil {
		log.Error().Err(err).Msg("task_respond_failed")
		return
	}

	entry := st.Task{
		UserID:     userID,
		MessageID:  msgID,
		AssignedAt: now,
		ExpiresAt:  expiry,
		Status:     st.TaskStatusPending,
	}
	// Bail if the record did not land: the timers below drive off it, and an
	// unrecorded task would leave the holder with a button and no state behind it.
	if err := store.SetTask(guildID, userID, entry); err != nil {
		log.Error().Str("guild_id", guildID).Str("user_id", userID).Err(err).Msg("task_store_failed")
		return
	}

	ctxTimer, cancel := context.WithCancel(context.Background())
	taskCancelMutex.Lock()
	taskCancels[userID] = cancel
	taskCancelMutex.Unlock()

	go handleTimers(log, ctx.API, store, ctxTimer, guildID, userID, ctx.ChannelID(), msgID, time.Until(expiry), reminderDelay)
}

func (c *TaskCommand) Component(ctx *adapter.ComponentInteractionContext) error {
	guildID := ctx.GuildID()
	userID := ctx.UserID()

	task, err := ctx.Storage.GetTask(guildID, userID)
	if err != nil || task == nil {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: "No active task found. Trying to cheat, hmm?",
			Color:       reply.EmbedColor,
		})
		return nil
	}

	if task.UserID != userID {
		_ = ctx.RespondEphemeral(&adapter.Embed{
			Description: "That task doesn't belong to you. Greedy little fingers, aren't you?",
			Color:       reply.EmbedColor,
		})
		return nil
	}

	if task.Status != st.TaskStatusPending {
		return ctx.ReplaceMessage(adapter.Reply{
			Text: ctx.MessageContent,
		})
	}

	switch ctx.ComponentID {
	case "task_complete_trigger":
		return ctx.ReplaceMessage(adapter.Reply{
			Text: ctx.MessageContent,
			Buttons: []adapter.ActionRow{{
				Buttons: []adapter.Button{
					{Label: "Yes", Style: adapter.SuccessButton, CustomID: "task_complete_yes"},
					{Label: "No", Style: adapter.DangerButton, CustomID: "task_complete_no"},
					{Label: "Safeword", Style: adapter.SecondaryButton, CustomID: "task_complete_safeword"},
				},
			}},
		})
	case "task_complete_yes", "task_complete_no", "task_complete_safeword":
		c.handleTaskCompletion(ctx, task)
	}

	return nil
}

func (c *TaskCommand) handleTaskCompletion(ctx *adapter.ComponentInteractionContext, task *st.Task) {
	userID, guildID := ctx.UserID(), ctx.GuildID()
	customID := ctx.ComponentID

	var msg string
	switch customID {
	case "task_complete_yes":
		task.Status = st.TaskStatusCompleted
		msg = "**Task Completed**\n" + fmt.Sprintf(randomLine(completeYesReplies), userID)
	case "task_complete_no":
		task.Status = st.TaskStatusFailed
		msg = "**Task Failed**\n" + fmt.Sprintf(randomLine(completeNoReplies), userID)
	case "task_complete_safeword":
		task.Status = st.TaskStatusSafeword
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

	if err := ctx.ReplaceMessage(adapter.Reply{Text: ctx.MessageContent}); err != nil {
		ctx.AppLog.Error().Str("custom_id", customID).Err(err).Msg("task_completion_ack_failed")
	}
	if err := ctx.FollowupWith(adapter.Reply{Text: msg}); err != nil {
		ctx.AppLog.Error().Str("custom_id", customID).Err(err).Msg("task_completion_followup_failed")
	}
}

// InitFromConfig loads the default task list from cfg.TasksPath. Call from main
// after loading config.
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

func handleTimers(log zerolog.Logger, api adapter.SessionAPI, storage *st.Storage, ctxTimer context.Context, guildID, userID, channelID, taskMsgID string, expiryDelay, reminderDelay time.Duration) {
	select {
	case <-time.After(reminderDelay):
		current, _ := storage.GetTask(guildID, userID)
		if current != nil && current.Status == st.TaskStatusPending {
			if err := api.SendChannelReply(channelID, taskMsgID,
				"**Task Reminder**\n"+fmt.Sprintf(randomLine(taskReminders), userID, humanDuration(expiryDelay-reminderDelay))); err != nil {
				log.Warn().Str("channel_id", channelID).Err(err).Msg("task_reminder_failed")
			}
		}
	case <-ctxTimer.Done():
		return
	}

	select {
	case <-time.After(expiryDelay - reminderDelay):
		current, _ := storage.GetTask(guildID, userID)
		if current != nil && current.Status == st.TaskStatusPending {
			if err := api.SendChannelReply(channelID, taskMsgID,
				"**Task Expired**\n"+fmt.Sprintf(randomLine(taskFailures), userID)); err != nil {
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
			if err := api.ClearChannelComponents(channelID, taskMsgID); err != nil {
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

func memberRoleNames(api adapter.SessionAPI, guildID string, roleIDs []string) (map[string]bool, error) {
	names := make(map[string]bool)
	if api == nil {
		return names, nil
	}
	byID, err := api.RoleNames(guildID)
	if err != nil {
		return names, err
	}
	for _, rid := range roleIDs {
		if name, ok := byID[rid]; ok {
			names[name] = true
		}
	}
	return names, nil
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
