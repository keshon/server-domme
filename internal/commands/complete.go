package commands

// import (
// 	"fmt"
// 	"math/rand"
// 	"server-domme/internal/storage"
// 	"time"

// 	"github.com/bwmarrin/discordgo"
// )

// var tasks = []string{
// 	"💋 Time to dance! Find a classic Backstreet Boys song and show me your best boy band moves.",
// 	"🍌 Eat a banana seductively and post the aftermath.",
// 	"📸 Take a selfie with your most bratty expression. Don’t hold back.",
// }

// var taskReminders = []string{
// 	"⏳ <@%s>, only %s left. You better be sweating, not slacking.\n\n>>> %s",
// 	"🕰️ <@%s>, tick-tock brat. %s and I’m judging.\n\n>>> %s",
// 	"🔥 <@%s>, the clock’s almost up. Impress me or regret me.\n\n>>> %s",
// 	"🎀 <@%s>, %s left. Wrap it up with style... or don't bother.\n\n>>> %s",
// 	"🐾 <@%s>, your time’s nearly up. Crawl faster, pet.\n\n>>> %s",
// }

// var taskFailures = []string{
// 	"💣 <@%s> let the clock win. I expected disappointment, and you *still* underdelivered.",
// 	"🧹 <@%s> swept their chance under the rug. Pathetic.",
// 	"📉 <@%s> failed. Again. Shock level: nonexistent.",
// 	"💤 <@%s> snoozed. Lost. Typical.",
// }

// var completeYesReplies = []string{
// 	"💎 <@%s> actually did it? Miracles happen. Pat yourself. I won’t.",
// 	"✨ <@%s>, for once you’re not a complete disappointment. Noted.",
// 	"😈 <@%s> obeyed. Good. You may bask in my fleeting approval.",
// }

// var completeNoReplies = []string{
// 	"👎 <@%s> admitted failure. At least you’re honest. Still useless.",
// 	"🪦 <@%s> buried the task. No tears here.",
// 	"🙃 <@%s> gave up. Try not to make it a habit. Oh wait…",
// }

// var completeSafewordReplies = []string{
// 	"⚠️ <@%s> used the safeword. Fine. I’ll let it slide... this time.",
// 	"🛑 <@%s> called mercy. Respect given, grudgingly.",
// 	"📉 <@%s> pulled the plug before the full flop. Good instincts.",
// }

// var (
// 	reminderDelay = 10 * time.Second
// 	expiryDelay   = 20 * time.Second
// )

// func init() {
// 	Register(&Command{
// 		Sort:               100,
// 		Name:               "task",
// 		Description:        "Assigns and manages your task",
// 		Category:           "Tasks",
// 		DCSlashHandler:     taskSlashHandler,
// 		DCComponentHandler: taskComponentHandler,
// 	})
// }

// func taskSlashHandler(ctx *SlashContext) {
// 	s, i := ctx.Session, ctx.Interaction

// 	userID := i.Member.User.ID
// 	guildID := i.GuildID

// 	if existing, _ := ctx.Storage.GetUserTask(guildID, userID); existing != nil && existing.Status == "pending" {
// 		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
// 			Type: discordgo.InteractionResponseChannelMessageWithSource,
// 			Data: &discordgo.InteractionResponseData{
// 				Content: "You already have a task, darling. Finish one before begging for more.",
// 				Flags:   1 << 6,
// 			},
// 		})
// 		return
// 	}

// 	task := tasks[rand.Intn(len(tasks))]
// 	now := time.Now()
// 	expiry := now.Add(expiryDelay)
// 	expiryText := humanDuration(expiryDelay)

// 	taskMsg := fmt.Sprintf(
// 		"<@%s> %s\n\nYou have %s to submit proof. Don’t disappoint me.\n\n*When you're done (or if you’re too weak to go on), respond below like a good little plaything.*",
// 		userID, task, expiryText)

// 	taskEntry := storage.UserTask{
// 		UserID:     userID,
// 		TaskText:   task,
// 		AssignedAt: now,
// 		ExpiresAt:  expiry,
// 		Status:     "pending",
// 	}

// 	ctx.Storage.SetUserTask(guildID, userID, taskEntry)

// 	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
// 		Type: discordgo.InteractionResponseChannelMessageWithSource,
// 		Data: &discordgo.InteractionResponseData{
// 			Content: taskMsg,
// 			Components: []discordgo.MessageComponent{
// 				discordgo.ActionsRow{
// 					Components: taskButtons(),
// 				},
// 			},
// 		},
// 	})

// 	go handleTimers(ctx, guildID, userID, i.ChannelID, taskMsg)
// }

// func handleTimers(ctx *SlashContext, guildID, userID, channelID, original string) {
// 	time.Sleep(reminderDelay)
// 	current, _ := ctx.Storage.GetUserTask(guildID, userID)
// 	if current != nil && current.Status == "pending" {
// 		reminder := fmt.Sprintf(randomLine(taskReminders), userID, humanDuration(expiryDelay-reminderDelay), current.TaskText)
// 		ctx.Session.ChannelMessageSend(channelID, reminder)
// 	}

// 	time.Sleep(expiryDelay - reminderDelay)
// 	current, _ = ctx.Storage.GetUserTask(guildID, userID)
// 	if current != nil && current.Status == "pending" {
// 		failMsg := fmt.Sprintf(randomLine(taskFailures), userID)
// 		ctx.Session.ChannelMessageSend(channelID, failMsg)
// 		ctx.Storage.ClearUserTask(guildID, userID)
// 	}
// }

// func taskComponentHandler(ctx *ComponentContext) {
// 	s, i := ctx.Session, ctx.Interaction
// 	userID := i.Member.User.ID
// 	guildID := i.GuildID
// 	storage := ctx.Storage

// 	task, err := storage.GetUserTask(guildID, userID)
// 	if err != nil || task == nil || task.Status != "pending" {
// 		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
// 			Type: discordgo.InteractionResponseUpdateMessage,
// 			Data: &discordgo.InteractionResponseData{
// 				Content:    "No active task found. Trying to cheat, hmm?",
// 				Components: []discordgo.MessageComponent{},
// 			},
// 		})
// 		return
// 	}

// 	var reply string
// 	switch i.MessageComponentData().CustomID {
// 	case "complete_yes":
// 		task.Status = "completed"
// 		reply = fmt.Sprintf(randomLine(completeYesReplies), userID)
// 	case "complete_no":
// 		task.Status = "failed"
// 		reply = fmt.Sprintf(randomLine(completeNoReplies), userID)
// 	case "complete_safeword":
// 		task.Status = "safeword"
// 		reply = fmt.Sprintf(randomLine(completeSafewordReplies), userID)
// 	default:
// 		reply = "Something went wrong. Try again, if your fingers aren’t too sweaty."
// 	}

// 	storage.ClearUserTask(guildID, userID)

// 	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
// 		Type: discordgo.InteractionResponseUpdateMessage,
// 		Data: &discordgo.InteractionResponseData{
// 			Content:    reply,
// 			Components: []discordgo.MessageComponent{},
// 		},
// 	})
// }

// func taskButtons() []discordgo.MessageComponent {
// 	return []discordgo.MessageComponent{
// 		discordgo.Button{Label: "Yes", Style: discordgo.SuccessButton, CustomID: "complete_yes"},
// 		discordgo.Button{Label: "No", Style: discordgo.DangerButton, CustomID: "complete_no"},
// 		discordgo.Button{Label: "Safeword", Style: discordgo.SecondaryButton, CustomID: "complete_safeword"},
// 	}
// }

// func randomLine(list []string) string {
// 	return list[rand.Intn(len(list))]
// }

// func humanDuration(d time.Duration) string {
// 	if d.Hours() >= 1 {
// 		return fmt.Sprintf("%d hour%s", int(d.Hours()), pluralize(int(d.Hours())))
// 	}
// 	if d.Minutes() >= 1 {
// 		return fmt.Sprintf("%d minute%s", int(d.Minutes()), pluralize(int(d.Minutes())))
// 	}
// 	return fmt.Sprintf("%d second%s", int(d.Seconds()), pluralize(int(d.Seconds())))
// }

// func pluralize(n int) string {
// 	if n == 1 {
// 		return ""
// 	}
// 	return "s"
// }
