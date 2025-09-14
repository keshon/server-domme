package command

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
func (c *TaskCommand) RequireAdmin() bool  { return false }
func (c *TaskCommand) RequireDev() bool    { return false }

func (c *TaskCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Type:        discordgo.ChatApplicationCommand,
	}
}
func (c *TaskCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*core.SlashContext)
	if !ok {
		return fmt.Errorf("wrong context")
	}

	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	guildID := event.GuildID
	member := event.Member
	userID := member.User.ID

	if cooldownUntil, err := storage.GetCooldown(guildID, userID); err == nil && time.Now().Before(cooldownUntil) {
		core.RespondEphemeral(session, event, fmt.Sprintf("Not so fast, darling. Wait **%s**.", humanDuration(time.Until(cooldownUntil))))
		return nil
	}

	if slices.Contains(config.New().ProtectedUsers, userID) {
		core.Respond(session, event, "You’re above this. No tasks for you.")
		return nil
	}

	taskCancelMutex.Lock()
	if cancel, exists := taskCancels[userID]; exists {
		cancel()
		delete(taskCancels, userID)
	}
	taskCancelMutex.Unlock()

	if existing, _ := storage.GetTask(guildID, userID); existing != nil && existing.Status == "pending" {
		core.RespondEphemeral(session, event, "One task at a time, sweetheart.")
		return nil
	}

	taskerRoles, _ := storage.GetTaskRoles(guildID)
	if len(taskerRoles) == 0 {
		core.RespondEphemeral(session, event, "No tasker roles set. So sad.")
		return nil
	}

	memberRoleNames := getMemberRoleNames(session, guildID, event.Member.Roles)
	tasks, err := loadTasksForGuild(guildID)
	if err != nil {
		core.RespondEphemeral(session, event, "Failed to load tasks.")
		log.Println("loadTasksForGuild:", err)
		return nil
	}

	filtered := filterTasksByRoles(tasks, memberRoleNames)
	if len(filtered) == 0 {
		core.RespondEphemeral(session, event, "No task suits your... profile.")
		return nil
	}

	task := filtered[rand.Intn(len(filtered))]
	c.assignTask(session, event, task, storage)

	err = core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func loadTasksForGuild(guildID string) ([]Task, error) {
	file := filepath.Join("data", fmt.Sprintf("%s_tasks.json", guildID))
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

func (c *TaskCommand) Component(ctx *core.ComponentContext) error {
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

func (c *TaskCommand) handleTaskCompletion(ctx *core.ComponentContext, event *discordgo.InteractionCreate, task *st.Task) {
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
		core.WithGroupAccessCheck()(
			core.WithGuildOnly(
				&TaskCommand{},
			),
		),
	)
}

var taskReminders = []string{
	"⏳ <@%s>, only %s left. You better be sweating, not slacking.",
	"🕰️ <@%s>, tick-tock brat. %s left and I’m judging.",
	"⏳ <@%s>, only %s left. You better be sweating, not slacking.",
	"🕰️ <@%s>, tick-tock brat. %s left and I’m judging.",
	"🔥 <@%s>, the clock’s almost up. %s to impress me or regret me.",
	"🎀 <@%s>, %s left. Wrap it up with style... or don't bother.",
	"🎀 <@%s>, %s left. Wrap it up with style... or don't bother.",
	"🐾 <@%s>, your time’s nearly up. %s to crawl faster, pet.",
	"👀 <@%s>, %s left. I’m watching… and I’m not impressed yet.",
	"🔪 <@%s>, %s left. Cut through the fear or bleed mediocrity.",
	"🍷 <@%s>, sip your shame now or earn a toast. You’ve got %session.",
	"🐍 <@%s>, slither faster. %s of mercy left.",
	"🧨 <@%s>, time’s ticking. %s to explode with effort or fade quietly.",
	"🖤 <@%s>, %s left to prove you're more than a waste of code.",
	"⚰️ <@%s>, %s to finish the task or bury your pride with it.",
	"💋 <@%s>, still dragging your heels? %s left. Hustle, slut.",
	"🎬 <@%s>, %s left. Deliver drama or stay irrelevant.",
	"🐖 <@%s>, move that lazy ass. %s isn’t a suggestion.",
	"💼 <@%s>, deadlines don’t beg. But I might… if you’re *very* good. %s left.",
	"🧁 <@%s>, sweetie, I’d bake you a reward if you earned it. You have %session.",
	"🎭 <@%s>, the final act begins. %s to avoid tripping over your mediocrity.",
	"🎯 <@%s>, bullseye or bust. You’ve got %s to not embarrass me.",
	"🔔 <@%s>, consider this your final bell. %s to deliver or get devoured.",
	"🐾 <@%s>, finish crawling. %s before your leash tightens further.",
	"🗡️ <@%s>, you’ve got %s to stab the task or stab your pride.",
	"🦴 <@%s>, fetch the result. %s left and I’m not throwing again.",
	"📉 <@%s>, productivity’s falling. %s left to fake competence.",
	"⛓️ <@%s>, tighten up. %s before I tighten the chain.",
	"🐇 <@%s>, tick-tock, Alice. %s to go down the hole or out of my sight.",
	"💦 <@%s>, don’t leak panic yet. %s left to make me purr.",
	"💭 <@%s>, still daydreaming? Snap out of it. %s to act.",
	"🧃 <@%s>, juice it or lose it. %s left. The clock isn’t fond of slackers.",
	"🕳️ <@%s>, finish what you started. Or should I finish *you* instead in %s?",
	"🐈 <@%s>, curiosity dies in %session. Better show me something worth watching.",
	"💃 <@%s>, shake it like time’s almost gone — %s left.",
	"🌪️ <@%s>, the storm's coming. %s to finish or get swept out like trash.",
}

var taskFailures = []string{
	"🧹 <@%s> swept their chance under the rug. Pathetic.",
	"📉 <@%s> failed. Again. Shock level: nonexistent.",
	"💤 <@%s> snoozed. Lost. Typical.",
	"🥀 <@%s> wilted under pressure. How predictably boring.",
	"🕳️ <@%s> disappeared when it mattered. How very on-brand.",
	"💩 <@%s> left a mess and called it effort. No thank you.",
	"🐌 <@%s> moved at a snail's pace and got exactly what they deserved. Nothing.",
	"🍂 <@%s> crumbled like a dry leaf. Blow away already.",
	"🛑 <@%s> didn’t even reach the line, let alone cross it.",
	"🐓 <@%s> chickened out. Knew you would.",
	"💔 <@%s> broke my patience. You had one job.",
	"🧊 <@%s> froze up. And now? Ice cold silence.",
	"🗑️ <@%s> submitted nothing. Trash takes itself out.",
	"🦴 <@%s> dropped the bone. No fetch, no treat.",
	"🎈 <@%s> floated away into irrelevance. Pathetic.",
	"🐀 <@%s> scurried off and left the task to rot.",
	"📵 <@%s> ghosted their own deadline. Tragic.",
	"🧠 <@%s> forgot the task. Or forgot their brain.",
	"🚫 <@%s> didn't even try. The absence is louder than your effort.",
	"🧻 <@%s> flushed the whole task. And dignity, apparently.",
	"🥱 <@%s> yawned through the hour. Now I’m yawning at *you*.",
	"🚽 <@%s> dropped the ball straight into the toilet.",
	"🐮 <@%s> stood there like a cow in headlights. Moo-ve on.",
	"🐤 <@%s> didn’t hatch anything useful. Just warm failure.",
	"🧂 <@%s> is salty, not spicy. Boring and bland.",
	"📪 <@%s> left their task undelivered. Return to sender, loser.",
	"🪦 <@%s> buried the chance deep. No flowers on this grave.",
	"🪰 <@%s> buzzed around and accomplished nothing. Swatted.",
	"🍕 <@%s> ordered failure with extra cheese. Served cold.",
	"🍷 <@%s> aged poorly. Time was not your friend.",
	"🧟 <@%s> lifeless effort. Undead, uninspired, unwanted.",
	"👻 <@%s> vanished. Not spooky. Just spineless.",
}

var completeYesReplies = []string{
	"💎 <@%s> actually did it? Miracles happen. Pat yourself. I won’t.",
	"✨ <@%s>, for once you’re not a complete disappointment. Noted.",
	"😈 <@%s> obeyed. Good. You may bask in my fleeting approval.",
	"🎉 <@%s> pulled it off. Don’t let it go to your empty little head.",
	"👏 <@%s> did the thing. Finally. Minimal praise granted.",
	"🌟 <@%s>, look at you. Functioning like a decent human. Rare.",
	"💼 <@%s> completed their task. I almost care.",
	"🥂 <@%s> managed success. I’m mildly impressed. Barely.",
	"🧠 <@%s> used their brain. I know, I’m shocked too.",
	"🚀 <@%s> launched into competence. Don’t crash it now.",
	"🪄 <@%s> managed to impress me. Once. Record it.",
	"📈 <@%s> is trending upward. Until you inevitably spiral.",
	"🔥 <@%s>, success looks… tolerable on you.",
	"👑 <@%s> gets a crown today. Paper. Temporary.",
	"🧹 <@%s> cleaned up their mess for once. Good pet.",
	"🫦 <@%s>, you did as told. That's hot. Shame it's rare.",
	"🪙 <@%s> earned something today. Don’t get used to it.",
	"📚 <@%s> followed instructions. Reading comprehension unlocked.",
	"🧸 <@%s>, you were a good little thing. Just this once.",
	"🥇 <@%s> won the bare minimum medal. Hang it in shame.",
	"🧬 <@%s> proved evolution isn’t fake. Just slow in your case.",
	"💌 <@%s>, I noticed. Don’t expect affection. Just acknowledgment.",
	"🔓 <@%s> unlocked mild favor. Don’t spend it all at once.",
	"📦 <@%s> delivered. Don’t worry, I won’t sign for it.",
	"🍒 <@%s> popped their competence cherry. Finally.",
	"🥵 <@%s>, seeing you obey? Unexpectedly hot.",
	"🛎️ <@%s> rang the bell of success. I may or may not answer.",
	"🪞 <@%s> looked responsibility in the eye… and didn’t flinch.",
	"💋 <@%s> kissed failure goodbye. For now.",
	"🧊 <@%s> kept it cool and did it right. Who even are you?",
	"🌹 <@%s>, that was… pleasant. Gross. But well done.",
	"🪄 <@%s> waved their magic brain cell and won.",
	"🎓 <@%s> graduated from Failure Academy. Cum less than laude.",
}

var completeNoReplies = []string{
	"🙄 <@%s> failed. Again. Why am I not surprised?",
	"💔 <@%s> couldn’t manage the simplest task. Useless.",
	"😒 <@%s> flopped like a sad little fish. No coins. Just shame.",
	"🗑️ <@%s> tossed effort out the window. Straight into the bin.",
	"😬 <@%s> choked harder than expected. And not in the good way.",
	"🎯 <@%s> missed the mark by a galaxy. Tragic.",
	"📉 <@%s> continues their downward spiral. Majestic in its failure.",
	"🚫 <@%s> chose to suck. Bold choice. Poor result.",
	"🫠 <@%s> melted under pressure. Lukewarm at best.",
	"🐌 <@%s> moved slower than ambition. Result: nothing.",
	"🪦 <@%s>'s task? Dead. Buried. Forgotten.",
	"🚽 <@%s> flushed success away. Bravo, toilet gremlin.",
	"🥀 <@%s> wilted under the weight of a basic ask.",
	"📎 <@%s> was attached to failure like a bad résumé.",
	"🛑 <@%s>, maybe just stop trying. It’s embarrassing.",
	"💤 <@%s> slept through responsibility. Again.",
	"🤡 <@%s> performed, but the circus was canceled.",
	"🎢 <@%s> had highs and lows. Mostly lows.",
	"🕳️ <@%s> fell short. Then tripped on their own excuse.",
	"🪰 <@%s> buzzed around the task, never landed on it.",
	"🛠️ <@%s> broke the task. And my faith in you.",
	"🎈 <@%s> floated away from expectations. Pop.",
	"🐴 <@%s> couldn’t drag themselves to the finish line. Pathetic.",
	"📺 <@%s>'s failure was broadcast live. Ratings: zero.",
	"💀 <@%s> killed it. But like, in the worst way.",
	"🌪️ <@%s> brought chaos, not completion.",
	"🧻 <@%s> wiped out before they even started.",
	"🧱 <@%s> ran into a wall made of their own incompetence.",
	"👣 <@%s> took one step forward, two into failure.",
	"🧊 <@%s> froze and shattered. Cleanup aisle 3.",
	"📦 <@%s> delivered disappointment. Again.",
	"🔕 <@%s> went silent when it mattered. Classic.",
	"🪤 <@%s> fell into the trap of not trying. Predictable.",
}

var completeSafewordReplies = []string{
	"⚠️ <@%s> used the safeword. Fine. I’ll let it slide... this time.",
	"🛑 <@%s> called mercy. Respect given, grudgingly.",
	"💤 <@%s> tapped out. Task canceled. Consent above all, darling.",
	"🧷 <@%s> knew their limit and spoke up. That’s rare. And smart.",
	"📉 <@%s> pulled the plug before the full flop. Good instincts.",
	"🕊️ <@%s> asked for peace. Fine. But don’t make it a habit.",
	"🎗️ <@%s> chose self-preservation. I *guess* I’ll allow it.",
	"🔐 <@%s> closed the door on the task. Consent first. Always.",
	"🫧 <@%s> slipped away under the safeword. You live—for now.",
	"🪫 <@%s> ran out of power. I won’t recharge you, but okay.",
	"📵 <@%s> disconnected. Silent mode activated. Noted.",
	"🚪 <@%s> exited the game. Voluntary retreat. Respect.",
	"🧘 <@%s> chose calm over chaos. Uncharacteristically wise.",
	"🌫️ <@%s> vanished into the safeword mist. Dramatic little thing.",
	"🧦 <@%s> pulled the emergency sock. I suppose I’ll let go.",
	"🧱 <@%s> hit their limit wall. And actually admitted it.",
	"🧩 <@%s> didn’t fit the task this time. That’s okay. I guess.",
	"🛋️ <@%s> retreated to their safe space. Plush and quiet. Like them.",
	"🌀 <@%s> spiraled, then called timeout. Clean exit.",
	"📪 <@%s> returned the challenge unopened. I’ll sign the receipt.",
	"🫱 <@%s> raised the flag. Not white, more... pearl-pink.",
	"🩹 <@%s> needed a breather. Consider it granted.",
	"📍 <@%s> pinned the limit. You’re learning. Slowly.",
	"🔮 <@%s> foresaw disaster and bailed. Smart brat.",
	"📯 <@%s> blew the horn of surrender. Echoes noted.",
	"🪞 <@%s> saw themselves losing it and hit pause. Growth?",
	"💿 <@%s> ejected mid-task. I won’t press play again. Yet.",
	"🩷 <@%s> protected themselves. Proud? Maybe.",
	"🧤 <@%s> tapped out with style. Respect where it's due.",
	"📷 <@%s> didn’t finish, but knew when to say stop. That's rare.",
	"🌡️ <@%s> reached boiling point and chose dignity. Brave move.",
	"🚷 <@%s> set boundaries. Look at you, developing a spine.",
	"⛓️ <@%s> broke the chain with a whisper. I'll allow it.",
}
