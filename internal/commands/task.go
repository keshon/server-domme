package commands

import (
	"fmt"
	"math/rand"
	"server-domme/internal/storage"
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
	"🔥 <@%s>, the clock’s almost up. Impress me or regret me.",
	"🎀 <@%s>, 10 minutes left. Wrap it up with style... or don't bother.",
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
	"💣 <@%s> let the clock win. I expected disappointment, and you *still* underdelivered.",
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

var (
	reminderDelay = 50 * time.Minute // default: 10 minutes before expiry (60m - 10m)
	expiryDelay   = 60 * time.Minute // default: 1 hour total task time
)

func init() {
	reminderDelay = 10 * time.Second // for quick tests, 10 seconds left warning
	expiryDelay = 20 * time.Second   // total 20 seconds task life for quick turnaround

	Register(&Command{
		Sort:           100,
		Name:           "task",
		Description:    "Assign a random task to the user",
		Category:       "Tasks",
		DCSlashHandler: taskSlashHandler,
	})
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
	expiryText := humanDuration(expiryDelay)
	taskText := fmt.Sprintf(
		"<@%s> %s\n\nYou have %s to submit proof. Don’t disappoint me.\n\n*When you're done (or if you’re too weak to go on), use* `/complete` *to turn it in, cancel, or safeword like the obedient thing you are.*",
		userID, task, expiryText)

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
		ExpiresAt:  now.Add(expiryDelay),
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
		time.Sleep(reminderDelay)
		current, err := ctx.Storage.GetUserTask(guildID, userID)
		if err == nil && current != nil && current.Status == "pending" {
			reminder := fmt.Sprintf(taskReminders[rand.Intn(len(taskReminders))], userID)
			s.ChannelMessageSend(i.ChannelID, reminder)
		}

		time.Sleep(expiryDelay - reminderDelay)
		current, err = ctx.Storage.GetUserTask(guildID, userID)
		if err == nil && current != nil && current.Status == "pending" {
			failMsg := fmt.Sprintf(taskFailures[rand.Intn(len(taskFailures))], userID)
			s.ChannelMessageSend(i.ChannelID, failMsg)
			ctx.Storage.ClearUserTask(guildID, userID)
		}
	}()

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
