package commands

import (
	"fmt"
	"math/rand"
	"slices"

	"github.com/bwmarrin/discordgo"
)

var punishPhrases = []string{
	"🔒 <@%s> has been sent to the Brat Corner. Someone finally found the line and crossed it.",
	"🚷 <@%s> has been escorted to the Brat Corner—with attitude still intact, unfortunately.",
	"🪑 <@%s> is now in time-out. Yes, again. No, we’re not negotiating.",
	"📢 <@%s> has been silenced with sass. Brat Corner echoes with regrets.",
	"🧼 <@%s>'s mouth was too dirty. Now they're washing it out in the Brat Corner.",
	"📦 <@%s> has been packaged and shipped directly to the Brat Corner. No returns.",
	"🫣 <@%s> thought they were cute. The Brat Corner disagrees.",
	"🥇 <@%s> won gold in the Olympic sport of testing my patience. Off you go.",
	"🎭 <@%s> put on quite the performance... now go act right in the Brat Corner.",
	"🚨 <@%s> triggered the ‘Too Much Mouth’ alarm. Detained accordingly.",
	"🛑 <@%s>, you’ve reached your limit. Please exit to the Brat Corner.",
	"🔇 <@%s> has been muted by the Ministry of Domme Affairs. Brat Corner-bound.",
	"🫦 <@%s> bit off more than they could brat. Corner time, sweetling.",
	"🧂 <@%s> was too salty to handle. Tossed in the Brat Corner to marinate.",
	"🎯 <@%s> made themselves a target. Direct hit—Brat Corner.",
	"💅 <@%s> was serving attitude. Now serving sentence. In the Brat Corner.",
	"🍑 <@%s>'s behavior? Spanked metaphorically. Now seated accordingly.",
	"🕰️ <@%s> needed a time-out. Forever, preferably.",
	"📉 <@%s>'s respect levels dropped below tolerable. Brat Corner it is.",
	"👶 <@%s> cried ‘unfair.’ Aww. Brat Corner’s got tissues and reality checks.",
	"🍵 <@%s> spilled too much tea and not enough sense. Time to steep in the Brat Corner.",
	"📖 <@%s>, your brat chapter just ended. The Brat Corner is the epilogue.",
	"🥄 <@%s> stirred too much. Now simmering alone.",
	"🎀 <@%s> looked cute doing wrong. Now look cute doing time.",
	"🧯 <@%s> got too hot to handle. Cooling off in the Brat Corner.",
	"📸 <@%s> caught in 4K acting up. Evidence archived. Sentence delivered.",
	"🫥 <@%s> vanished from good graces. Brat Corner: their last known location.",
	"🎲 <@%s> gambled with attitude and lost. Roll again in the Brat Corner.",
	"📌 <@%s> has been pinned for public shaming. Brat Corner is the display case.",
	"🕳️ <@%s>, dig yourself out—if you can. Brat Corner’s got depth.",
	"🛋️ <@%s> is now grounded. Emotionally. Spiritually. Physically. Permanently.",
	"📺 <@%s> is now broadcasting from the Brat Corner Live. Audience: none.",
	"🪤 <@%s> walked right into it. Classic brat trap.",
	"📎 <@%s> has been attached to the Brat Report. Filed under: Hopeless.",
}

func init() {
	Register(&Command{
		Sort:           200,
		Name:           "punish",
		Description:    "Punish a brat (assigns them the Brat Corner role)",
		Category:       "Moderation",
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

func buildPunishAction(s *discordgo.Session, guildID, targetID, victimRoleID string) (string, error) {
	err := s.GuildMemberRoleAdd(guildID, targetID, victimRoleID)
	if err != nil {
		return "", err
	}

	phrase := punishPhrases[rand.Intn(len(punishPhrases))]
	return fmt.Sprintf(phrase, targetID), nil
}

func punishSlashHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.Interaction, ctx.Storage
	options := i.ApplicationCommandData().Options

	punisherRoleID, err := storage.GetRoleForGuild(i.GuildID, "punisher")
	if err != nil || punisherRoleID == "" {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Hmm, no 'punisher' role configured yet. Tsk. Someone skipped setup.",
				Flags:   1 << 6,
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
				Flags:   1 << 6,
			},
		})
		return
	}

	if !slices.Contains(i.Member.Roles, punisherRoleID) {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Nice try, sugar. You don’t wear the right collar to give punishments.",
				Flags:   1 << 6,
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
				Flags:   1 << 6,
			},
		})
		return
	}

	msg, err := buildPunishAction(s, i.GuildID, targetUserID, victimRoleID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Tried to punish them, but they squirmed away: ```%v```", err),
				Flags:   1 << 6,
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
