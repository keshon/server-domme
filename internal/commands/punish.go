package commands

import (
	"fmt"
	"log"
	"math/rand"
	"server-domme/internal/config"
	"slices"

	"github.com/bwmarrin/discordgo"
)

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

	punisherRoleID, err := storage.GetPunishRole(i.GuildID, "punisher")
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

	victimRoleID, err := storage.GetPunishRole(i.GuildID, "victim")
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

	assignedRoleID, err := storage.GetPunishRole(i.GuildID, "assigned")
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

	guildID := i.GuildID
	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "punish")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}
