package command

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"slices"
	"sync"
	"time"

	"server-domme/internal/config"

	st "server-domme/internal/storagetypes"

	"github.com/bwmarrin/discordgo"
)

type TaskCommand struct {
	cfg        *config.Config
	tasks      []Task
	cancelMap  map[string]context.CancelFunc
	cancelLock *sync.Mutex
}

type Task struct {
	Description  string
	DurationMin  int
	RolesAllowed []string
}

func NewTaskCommand() *TaskCommand {
	cfg := config.New()
	taskList, err := loadTasks(cfg.TasksPath)
	if err != nil {
		log.Println("Failed to load tasks:", err)
	}
	if len(taskList) == 0 {
		log.Println("No tasks loaded!")
	}

	return &TaskCommand{
		cfg:        cfg,
		tasks:      taskList,
		cancelMap:  make(map[string]context.CancelFunc),
		cancelLock: &sync.Mutex{},
	}
}

func (t *TaskCommand) Name() string        { return "task" }
func (t *TaskCommand) Description() string { return "Assign or manage your personal task, slave" }
func (t *TaskCommand) Category() string    { return "🎭 Roleplay" }
func (t *TaskCommand) Aliases() []string   { return nil }

func (t *TaskCommand) RequireAdmin() bool { return false }
func (t *TaskCommand) RequireDev() bool   { return false }

func (t *TaskCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        t.Name(),
		Description: t.Description(),
		Type:        discordgo.ChatApplicationCommand,
	}
}

func (t *TaskCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("не тот тип контекста")
	}

	session, event, storage := slash.Session, slash.Event, slash.Storage
	userID, guildID := event.Member.User.ID, event.GuildID

	if cdUntil, err := storage.GetCooldown(guildID, userID); err == nil && time.Now().Before(cdUntil) {
		left := time.Until(cdUntil)
		_ = respondEphemeral(session, event, fmt.Sprintf("Not so fast, darling. Wait **%s** before I play again.", humanDuration(left)))
		return nil
	}

	if slices.Contains(t.cfg.ProtectedUsers, userID) {
		_ = respond(session, event, "Some lines even a Domme bot won’t cross. No tasks for the one who commands the code.")
		return nil
	}

	t.cancelLock.Lock()
	if cancel, exists := t.cancelMap[userID]; exists {
		cancel()
		delete(t.cancelMap, userID)
	}
	t.cancelLock.Unlock()

	if existing, _ := storage.GetUserTask(guildID, userID); existing != nil && existing.Status == "pending" {
		_ = respondEphemeral(session, event, "You already have a task. Finish one before begging for more.")
		return nil
	}

	roleIDs, err := storage.GetTaskRoles(guildID)
	if err != nil || len(roleIDs) == 0 {
		_ = respondEphemeral(session, event, "No tasker roles set. I can’t give out toys to just anyone.")
		return nil
	}

	roleMap := getMemberRoleNames(session, guildID, event.Member.Roles)
	filtered := filterTasksByRoles(t.tasks, roleMap)
	if len(filtered) == 0 {
		_ = respondEphemeral(session, event, "None of the tasks fit your… limited skill set.")
		return nil
	}

	selected := filtered[rand.Intn(len(filtered))]
	t.assignTask(slash, selected)
	return nil
}

func (t *TaskCommand) assignTask(slash *SlashContext, task Task) {
	session, event, storage := slash.Session, slash.Event, slash.Storage
	userID, guildID := event.Member.User.ID, event.GuildID
	now := time.Now()
	expiry := now.Add(time.Duration(task.DurationMin) * time.Minute)
	reminderDelay := time.Duration(float64(task.DurationMin)*0.9) * time.Minute

	msg := fmt.Sprintf("**New Task**\n<@%s> %s\n\n*You have %s to complete this task.*", userID, task.Description, humanDuration(time.Until(expiry)))

	err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
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
		log.Println("Failed to send task:", err)
		return
	}

	savedMsg, err := session.InteractionResponse(event.Interaction)
	if err != nil {
		log.Println("Failed to get response msg:", err)
		return
	}

	entry := st.UserTask{
		UserID:     userID,
		MessageID:  savedMsg.ID,
		AssignedAt: now,
		ExpiresAt:  expiry,
		Status:     "pending",
	}
	_ = storage.SetUserTask(guildID, userID, entry)

	ctxTimer, cancel := context.WithCancel(context.Background())
	t.cancelLock.Lock()
	t.cancelMap[userID] = cancel
	t.cancelLock.Unlock()

	go t.handleTimers(slash, ctxTimer, guildID, userID, event.ChannelID, savedMsg.ID, time.Until(expiry), reminderDelay)
}

func (t *TaskCommand) handleTimers(ctx *SlashContext, cancelCtx context.Context, guildID, userID, channelID, msgID string, expire, remind time.Duration) {
	select {
	case <-time.After(remind):
		current, _ := ctx.Storage.GetUserTask(guildID, userID)
		if current != nil && current.Status == "pending" {
			ctx.Session.ChannelMessageSend(channelID, fmt.Sprintf("**Task Reminder**\n<@%s> %s", userID, humanDuration(expire-remind)))
		}
	case <-cancelCtx.Done():
		return
	}

	select {
	case <-time.After(expire - remind):
		current, _ := ctx.Storage.GetUserTask(guildID, userID)
		if current != nil && current.Status == "pending" {
			ctx.Storage.ClearUserTask(guildID, userID)
			ctx.Storage.SetCooldown(guildID, userID, time.Now().Add(3*time.Hour))
			ctx.Session.ChannelMessageSend(channelID, fmt.Sprintf("**Task Expired**\n<@%s> Failed.", userID))
		}
	case <-cancelCtx.Done():
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

func getMemberRoleNames(s *discordgo.Session, guildID string, roleIDs []string) map[string]bool {
	names := make(map[string]bool)
	for _, rid := range roleIDs {
		role, err := s.State.Role(guildID, rid)
		if err != nil || role == nil {
			allRoles, _ := s.GuildRoles(guildID)
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

func init() {
	Register(WithGuildOnly(NewTaskCommand()))
}
