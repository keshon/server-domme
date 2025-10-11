package task

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"server-domme/internal/config"
	"server-domme/internal/core"
	"server-domme/internal/storage"
	"slices"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	st "server-domme/internal/storagetypes"
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

type TaskCommand struct{}

func (c *TaskCommand) Name() string        { return "task" }
func (c *TaskCommand) Description() string { return "Assign or manage your personal task" }
func (c *TaskCommand) Aliases() []string   { return []string{} }
func (c *TaskCommand) Group() string       { return "task" }
func (c *TaskCommand) Category() string    { return "🎭 Roleplay" }
func (c *TaskCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *TaskCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "self-assign",
				Description: "Assign yourself a new random task",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "manage",
				Description: "Manage task-related settings",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "set-role",
						Description: "Set or update a Tasker role",
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionRole,
								Name:        "role",
								Description: "Select the role allowed to get tasks",
								Required:    true,
							},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "list-role",
						Description: "List all task-related roles",
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "reset-role",
						Description: "Reset the Tasker role configuration",
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "upload-tasks",
						Description: "Upload a new task list for this server",
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionAttachment,
								Name:        "file",
								Description: "JSON file (.json) containing the task list",
								Required:    true,
							},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "download-tasks",
						Description: "Download the current task list for this server",
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "reset-tasks",
						Description: "Reset the task list to default for this server",
					},
				},
			},
		},
	}
}

func (c *TaskCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	storage := context.Storage

	data := e.ApplicationCommandData()

	if len(data.Options) == 0 {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	opt := data.Options[0]

	switch opt.Type {
	case discordgo.ApplicationCommandOptionSubCommand:
		// top-level subcommand
		switch opt.Name {
		case "self-assign":
			return c.runSelfAssign(context)
		default:
			return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Unknown subcommand.",
			})
		}

	case discordgo.ApplicationCommandOptionSubCommandGroup:
		// subcommand group
		switch opt.Name {
		case "manage":
			if len(opt.Options) == 0 {
				return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
					Description: "No subcommand provided under manage.",
				})
			}
			sub := opt.Options[0]
			return c.runManage(s, e, storage, sub)
		default:
			return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Unknown subcommand group.",
			})
		}

	default:
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Invalid or missing subcommand.",
		})
	}
}

func (c *TaskCommand) runSelfAssign(context *core.SlashInteractionContext) error {

	session := context.Session
	event := context.Event
	storage := context.Storage

	guildID := event.GuildID
	member := event.Member
	userID := member.User.ID

	if cooldownUntil, err := storage.GetCooldown(guildID, userID); err == nil && time.Now().Before(cooldownUntil) {
		core.RespondEphemeral(session, event, fmt.Sprintf("Not so fast, darling. Wait **%s**.", humanDuration(time.Until(cooldownUntil))))
		return nil
	}

	if slices.Contains(config.New().ProtectedUsers, userID) {
		core.Respond(session, event, "You're above this. No tasks for you.")
		return nil
	}

	taskCancelMutex.Lock()
	if cancel, exists := taskCancels[userID]; exists {
		cancel()
		delete(taskCancels, userID)
	}
	taskCancelMutex.Unlock()

	existing, _ := storage.GetTask(guildID, userID)
	if existing != nil && existing.Status == "pending" {
		core.RespondEphemeral(session, event, "One task at a time, sweetheart.")
		return nil
	}

	taskerRoles, _ := storage.GetTaskRole(guildID)
	if len(taskerRoles) == 0 {
		core.RespondEphemeral(session, event, "No tasker roles set. So sad.\n\nAsk an Admin to set them.")
		return nil
	}

	memberRoleNames := getMemberRoleNames(session, guildID, event.Member.Roles)
	tasks, err := loadTasksForGuild(guildID)
	if err != nil {
		core.RespondEphemeral(session, event, "Failed to load tasks.\n\nAsk an Admin to set them.")
		log.Println("loadTasksForGuild:", err)
		return nil
	}

	filtered := filterTasksByRoles(tasks, memberRoleNames)
	if len(filtered) == 0 {
		core.RespondEphemeral(session, event, "No task suits your... profile.\n\nAsk an Admin to upload tasks for your gender role and try again.")
		return nil
	}

	task := filtered[rand.Intn(len(filtered))]
	c.assignTask(session, event, task, storage)

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

func (c *TaskCommand) assignTask(session *discordgo.Session, event *discordgo.InteractionCreate, task Task, storage *storage.Storage) {
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
		log.Println("Failed to send task response:", err)
		return
	}

	msg, err := session.InteractionResponse(event.Interaction)
	if err != nil {
		log.Println("Failed to fetch interaction response:", err)
		return
	}

	entry := st.Task{
		UserID:     userID,
		MessageID:  msg.ID,
		AssignedAt: now,
		ExpiresAt:  expiry,
		Status:     "pending",
	}
	storage.SetTask(guildID, userID, entry)

	ctxTimer, cancel := context.WithCancel(context.Background())
	taskCancelMutex.Lock()
	taskCancels[userID] = cancel
	taskCancelMutex.Unlock()

	go handleTimers(session, storage, ctxTimer, guildID, userID, event.ChannelID, msg.ID, time.Until(expiry), reminderDelay)

}

func (c *TaskCommand) Component(ctx *core.ComponentInteractionContext) error {
	session := ctx.Session
	event := ctx.Event
	guildID := event.GuildID
	userID := event.Member.User.ID

	task, err := ctx.Storage.GetTask(guildID, userID)
	if err != nil || task == nil {
		core.RespondEphemeral(session, event, "No active task found. Trying to cheat, hmm?")
		return nil
	}

	if task.UserID != userID {
		core.RespondEphemeral(session, event, "That task doesn’t belong to you. Greedy little fingers, aren't you?")
		return nil
	}

	if task.Status != "pending" {
		session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		})
		return nil
	}

	switch event.MessageComponentData().CustomID {
	case "task_complete_trigger":
		session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
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
		})
	case "task_complete_yes", "task_complete_no", "task_complete_safeword":
		c.handleTaskCompletion(ctx, event, task)
	}

	return nil
}

func (c *TaskCommand) handleTaskCompletion(ctx *core.ComponentInteractionContext, event *discordgo.InteractionCreate, task *st.Task) {
	session := ctx.Session
	userID, guildID := event.Member.User.ID, event.GuildID
	customID := event.MessageComponentData().CustomID

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

	ctx.Storage.ClearTask(guildID, userID)
	ctx.Storage.SetCooldown(guildID, userID, time.Now().Add(cooldownDuration))

	taskCancelMutex.Lock()
	if cancel, exists := taskCancels[userID]; exists {
		cancel()
		delete(taskCancels, userID)
	}
	taskCancelMutex.Unlock()

	session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    event.Message.Content,
			Components: []discordgo.MessageComponent{},
		},
	})
	session.FollowupMessageCreate(event.Interaction, false, &discordgo.WebhookParams{
		Content: reply,
	})
}

func init() {
	cfg := config.New()
	var err error
	tasks, err = loadTasks(cfg.TasksPath)
	if err != nil {
		log.Println("[ERR] Failed to load tasks:", err)
		return
	}
	if len(tasks) == 0 {
		log.Println("[WARN] No tasks loaded! Aborting task assignment.")
		return
	}

	log.Printf("[INFO] Loaded %d tasks from %s\n", len(tasks), cfg.TasksPath)
}

func handleTimers(session *discordgo.Session, storage *storage.Storage, ctxTimer context.Context, guildID, userID, channelID, taskMsgID string, expiryDelay, reminderDelay time.Duration) {
	select {
	case <-time.After(reminderDelay):
		current, _ := storage.GetTask(guildID, userID)
		if current != nil && current.Status == "pending" {
			session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
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
		current, _ := storage.GetTask(guildID, userID)
		if current != nil && current.Status == "pending" {
			session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
				Content: "**Task Expired**\n" + fmt.Sprintf(randomLine(taskFailures), userID),
				Reference: &discordgo.MessageReference{
					MessageID: taskMsgID, ChannelID: channelID, GuildID: guildID,
				},
			})
			storage.ClearTask(guildID, userID)
			storage.SetCooldown(guildID, userID, time.Now().Add(cooldownDuration))
			session.ChannelMessageEditComplex(&discordgo.MessageEdit{
				ID: taskMsgID, Channel: channelID, Components: &[]discordgo.MessageComponent{},
			})
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

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&TaskCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
