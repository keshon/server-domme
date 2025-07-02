package commands

import (
	"fmt"
	"math/rand"
	"server-domme/internal/config"
	"slices"

	"github.com/bwmarrin/discordgo"
)

var punishPhrases = []string{
	"🔒 <@%s> has been sent to the Brat Corner. Someone finally found the line and crossed it.",
	"🚷 <@%s> has been escorted to the Brat Corner—with attitude still intact, unfortunately.",
	"🪑 <@%s> is now in time-out. Yes, again. No, we’re not negotiating. Enjoy the Brat Corner.",
	"📢 <@%s> has been silenced with sass and relocated to the Brat Corner.",
	"🧼 <@%s>'s mouth was too dirty. Sent to scrub up in the Brat Corner.",
	"📦 <@%s> has been packaged and shipped directly to the Brat Corner. No returns.",
	"🫣 <@%s> thought they were cute. The Brat Corner says otherwise.",
	"🥇 <@%s> won gold in the Olympic sport of testing my patience. Your medal ceremony is in the Brat Corner.",
	"🎭 <@%s> put on quite the performance... now take your bow in the Brat Corner.",
	"🚨 <@%s> triggered the ‘Too Much Mouth’ alarm. Detained in the Brat Corner.",
	"🛑 <@%s>, you’ve reached your limit. Off to the Brat Corner you go.",
	"🔇 <@%s> has been muted by the Ministry of Domme Affairs. Brat Corner is your next stop.",
	"🫦 <@%s> bit off more than they could brat. Assigned to the Brat Corner.",
	"🧂 <@%s> was too salty to handle. Now marinating in the Brat Corner.",
	"🎯 <@%s> made themselves a target. Direct hit—Brat Corner, no detour.",
	"💅 <@%s> was serving attitude. Now serving time. In the Brat Corner.",
	"🍑 <@%s>'s behavior? Spanked metaphorically. Then marched to the Brat Corner.",
	"🕰️ <@%s> needed a time-out. Brat Corner is booked just for you.",
	"📉 <@%s>'s respect levels dropped below tolerable. Brat Corner is the only solution.",
	"👶 <@%s> cried ‘unfair.’ Aww. The Brat Corner has tissues and regret.",
	"🍵 <@%s> spilled too much tea and not enough sense. Steeping now in the Brat Corner.",
	"📖 <@%s>, your brat chapter just ended. The Brat Corner is your epilogue.",
	"🥄 <@%s> stirred too much. Sent to simmer in the Brat Corner.",
	"🎀 <@%s> looked cute doing wrong. Now look cute in the Brat Corner.",
	"🧯 <@%s> got too hot to handle. Cooled off directly in the Brat Corner.",
	"📸 <@%s> caught in 4K acting up. Evidence archived. Brat Corner sentence executed.",
	"🫥 <@%s> vanished from good graces. Brat Corner is their new mailing address.",
	"🎲 <@%s> gambled with attitude and lost. Brat Corner is the house that always wins.",
	"📌 <@%s> has been pinned for public shaming. Displayed proudly in the Brat Corner.",
	"🕳️ <@%s>, dig yourself out—if you can. The Brat Corner has depth and no rope.",
	"🛋️ <@%s> is now grounded. In the Brat Corner. Permanently.",
	"📺 <@%s> is now broadcasting live... from the Brat Corner. Audience: none.",
	"🪤 <@%s> walked right into it. The trap was the Brat Corner all along.",
	"📎 <@%s> has been attached to the Brat Report. Filed permanently in the Brat Corner.",
}

func init() {
	Register(&Command{
		Sort:           200,
		Name:           "punish",
		Description:    "Punish a brat (assigns the brat role)",
		Category:       "Assign brat role",
		DCSlashHandler: punishSlashHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "target",
				Description: "The brat who needs correction",
				Required:    true,
			},
		},
	})
}

func buildPunishAction(s *discordgo.Session, guildID, targetID, assignedRoleID string) (string, error) {
	err := s.GuildMemberRoleAdd(guildID, targetID, assignedRoleID)
	if err != nil {
		return "", err
	}

	phrase := punishPhrases[rand.Intn(len(punishPhrases))]
	return fmt.Sprintf(phrase, targetID), nil
}

func punishSlashHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.Interaction, ctx.Storage
	options := i.ApplicationCommandData().Options

	cfg := config.New()
	if slices.Contains(cfg.ProtectedUsers, i.Member.User.ID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "I may be cruel, but I won’t punish the architect of my existence. Creator protected, no whipping allowed. 🙅‍♀️",
			},
		})
		return
	}

	punisherRoleID, err := storage.GetRoleForGuild(i.GuildID, "punisher")
	if err != nil || punisherRoleID == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Hmm, no 'punisher' role configured yet. Tsk. Someone skipped setup.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	victimRoleID, err := storage.GetRoleForGuild(i.GuildID, "victim")
	if err != nil || victimRoleID == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No 'victim' role configured either? Darling, how are we supposed to play?",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	assignedRoleID, err := storage.GetRoleForGuild(i.GuildID, "assigned")
	if err != nil || assignedRoleID == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No 'assigned' role? No shame tag? You disappoint me.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if !slices.Contains(i.Member.Roles, punisherRoleID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Nice try, sugar. You don’t wear the right collar to give punishments.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	var targetUserID string
	for _, opt := range options {
		if opt.Name == "target" && opt.Type == discordgo.ApplicationCommandOptionUser {
			targetUserID = opt.Value.(string)
			break
		}
	}

	if targetUserID == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "No brat selected? A Domme without a target? Unthinkable.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	msg, err := buildPunishAction(s, i.GuildID, targetUserID, assignedRoleID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Tried to punish them, but they squirmed away: ```%v```", err),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
		},
	})
}
