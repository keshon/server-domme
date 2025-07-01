package commands

import (
	"context"
	"fmt"
	"math/rand"
	"server-domme/internal/storage"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

var tasks = []string{
	"💋 Time to dance! Find a classic Backstreet Boys song and show me your best boy band moves.",
	"🍌 Eat a banana seductively and post the aftermath.",
	"📸 Take a selfie with your most bratty expression. Don’t hold back.",
}

var taskReminders = []string{
	"⏳ <@%s>, only 10 minutes left. You better be sweating, not slacking.",
	"🕰️ <@%s>, tick-tock brat. 10 minutes and I’m judging.",
	"⏳ <@%s>, only %s left. You better be sweating, not slacking.",
	"🕰️ <@%s>, tick-tock brat. %s and I’m judging.",
	"🔥 <@%s>, the clock’s almost up. Impress me or regret me.",
	"🎀 <@%s>, 10 minutes left. Wrap it up with style... or don't bother.",
	"🎀 <@%s>, %s left. Wrap it up with style... or don't bother.",
	"🐾 <@%s>, your time’s nearly up. Crawl faster, pet.",
	"👀 <@%s>, 10 minutes left. I’m watching… and I’m not impressed yet.",
	"🔪 <@%s>, 10 minutes. Cut through the fear or bleed mediocrity.",
	"🍷 <@%s>, sip your shame now or earn a toast. Your choice, darling.",
	"🐍 <@%s>, slither faster. Ten minutes of mercy left.",
	"🧨 <@%s>, time’s ticking. Explode with effort or fade quietly.",
	"🖤 <@%s>, 10 minutes left to prove you're more than a waste of code.",
	"⚰️ <@%s>, finish the task or bury your pride with it.",
	"💋 <@%s>, still dragging your heels? Ten minutes. Hustle, slut.",
	"🎬 <@%s>, 10 minutes. Deliver drama or stay irrelevant.",
	"🐖 <@%s>, move that lazy ass. 10 minutes isn’t a suggestion.",
	"💼 <@%s>, deadlines don’t beg. But I might… if you’re *very* good.",
	"🧁 <@%s>, sweetie, I’d bake you a reward if you earned it. You have 10 minutes.",
	"🎭 <@%s>, the final act begins. Don’t trip over your mediocrity.",
	"🎯 <@%s>, bullseye or bust. You’ve got 10 minutes to not embarrass me.",
	"🔔 <@%s>, consider this your final bell. Deliver or get devoured.",
	"🐾 <@%s>, finish crawling. Your leash is getting shorter.",
	"🗡️ <@%s>, you’ve got 10 minutes to stab the task or stab your pride.",
	"🦴 <@%s>, fetch the result. Time’s almost gone and I’m not throwing again.",
	"📉 <@%s>, productivity’s falling. 10 minutes left to fake competence.",
	"⛓️ <@%s>, tighten up. Ten minutes before I tighten the chain.",
	"🐇 <@%s>, tick-tock, Alice. Down the hole or out of my sight.",
	"💦 <@%s>, don’t leak panic yet. 10 minutes left to make me purr.",
	"💭 <@%s>, still daydreaming? Snap out of it. Ten minutes to act.",
	"🧃 <@%s>, juice it or lose it. The clock isn’t fond of slackers.",
	"🕳️ <@%s>, finish what you started. Or should I finish *you* instead?",
	"🐈 <@%s>, curiosity dies in 10 minutes. Better show me something worth watching.",
	"💃 <@%s>, shake it like time’s almost gone — because it is.",
	"🌪️ <@%s>, the storm's coming. Finish now or get swept out like trash.",
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

var (
	reminderDelay = 10 * time.Second
	expiryDelay   = 20 * time.Second
)

var taskCancels = make(map[string]context.CancelFunc)
var taskCancelMutex = sync.Mutex{}

func init() {
	Register(&Command{
		Sort:               100,
		Name:               "task",
		Description:        "Assigns and manages your task",
		Category:           "Tasks",
		DCSlashHandler:     taskSlashHandler,
		DCComponentHandler: taskComponentHandler,
	})
}

func taskSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.Interaction
	userID := i.Member.User.ID
	guildID := i.GuildID

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
				Flags:   1 << 6,
			},
		})
		return
	}

	task := tasks[rand.Intn(len(tasks))]
	now := time.Now()
	expiry := now.Add(expiryDelay)
	expiryText := humanDuration(expiryDelay)

	taskMsg := fmt.Sprintf(
		"<@%s> %s\n\n*You have %s to submit proof. Don’t disappoint me.\nWhen you're done (or if you’re too weak to go on), press the button.*",
		userID, task, expiryText)

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: taskMsg,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{Label: "Complete", Style: discordgo.PrimaryButton, CustomID: "task_complete_trigger"},
					},
				},
			},
		},
	})
	if err != nil {
		fmt.Println("Failed to send task response:", err)
		return
	}

	// 👇 This gets the message you just sent
	msg, err := s.InteractionResponse(i.Interaction)
	if err != nil {
		fmt.Println("Failed to fetch interaction response:", err)
		return
	}

	taskEntry := storage.UserTask{
		UserID:     userID,
		MessageID:  msg.ID,
		TaskText:   task,
		AssignedAt: now,
		ExpiresAt:  expiry,
		Status:     "pending",
	}
	ctx.Storage.SetUserTask(guildID, userID, taskEntry)

	ctxTimer, cancel := context.WithCancel(context.Background())

	taskCancelMutex.Lock()
	taskCancels[userID] = cancel
	taskCancelMutex.Unlock()

	go handleTimers(ctxTimer, ctx, guildID, userID, i.ChannelID, msg.ID)
}

func handleTimers(ctxTimer context.Context, ctx *SlashContext, guildID, userID, channelID, taskMsgID string) {
	select {
	case <-time.After(reminderDelay):
		current, _ := ctx.Storage.GetUserTask(guildID, userID)
		if current != nil && current.Status == "pending" {
			reminder := fmt.Sprintf(randomLine(taskReminders), userID, humanDuration(expiryDelay-reminderDelay))
			prefixedReminder := "**Reminder:** " + reminder
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
			prefixedFailMsg := "**Expired:** " + failMsg
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
	if err != nil || task == nil {
		// No task found
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "No active task found. Trying to cheat, hmm?",
				Components: []discordgo.MessageComponent{},
			},
		})
		return
	}

	if task.Status != "pending" {
		// Already completed
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
			reply = "**Completed:** " + reply

		case "task_complete_no":
			task.Status = "failed"
			reply = fmt.Sprintf(randomLine(completeNoReplies), userID)
			reply = "**Failed:** " + reply

		case "task_complete_safeword":
			task.Status = "safeword"
			reply = fmt.Sprintf(randomLine(completeSafewordReplies), userID)
			reply = "**Safeword:** " + reply
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
