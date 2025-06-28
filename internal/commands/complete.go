package commands

import (
	"fmt"
	"math/rand"

	"github.com/bwmarrin/discordgo"
)

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

func init() {
	Register(&Command{
		Sort:               101,
		Name:               "complete",
		Description:        "Respond to your active task with Yes, No, or Safeword",
		Category:           "Tasks",
		DCSlashHandler:     completeSlashHandler,
		DCComponentHandler: completeComponentHandler,
	})
}

func completeSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.Interaction

	userID := i.Member.User.ID
	guildID := i.GuildID
	storage := ctx.Storage

	task, err := storage.GetUserTask(guildID, userID)
	if err != nil || task == nil || task.Status != "pending" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No active task found. Are you lost, pet?",
				Flags:   1 << 6,
			},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("<@%s> Did you complete your task, slut? Be honest… or not.", userID),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{Label: "Yes", Style: discordgo.SuccessButton, CustomID: "complete_yes"}, // complete prefix MUST be equal to the command name
						discordgo.Button{Label: "No", Style: discordgo.DangerButton, CustomID: "complete_no"},
						discordgo.Button{Label: "Safeword", Style: discordgo.SecondaryButton, CustomID: "complete_safeword"},
					},
				},
			},
		},
	})
}

func completeComponentHandler(ctx *ComponentContext) {
	s, i := ctx.Session, ctx.Interaction
	userID := i.Member.User.ID
	guildID := i.GuildID
	storage := ctx.Storage

	task, err := storage.GetUserTask(guildID, userID)
	if err != nil || task == nil || task.Status != "pending" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No active task to respond to. Trying to fake your way out, hmm?",
				Flags:   1 << 6,
			},
		})
		return
	}

	var reply string
	customID := i.MessageComponentData().CustomID
	switch customID {
	case "complete_yes":
		task.Status = "completed"
		reply = fmt.Sprintf(randomLine(completeYesReplies), userID)
	case "complete_no":
		task.Status = "failed"
		reply = fmt.Sprintf(randomLine(completeNoReplies), userID)
	case "complete_safeword":
		task.Status = "safeword"
		reply = fmt.Sprintf(randomLine(completeSafewordReplies), userID)
	default:
		reply = "Unrecognized response. Are those fingers too clumsy?"
	}

	storage.ClearUserTask(guildID, userID)

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    reply,
			Components: []discordgo.MessageComponent{},
		},
	})
}

func randomLine(list []string) string {
	return list[rand.Intn(len(list))]
}
