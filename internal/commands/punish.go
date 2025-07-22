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
		Sort:           30,
		Name:           "punish",
		Description:    "Assign the brat role for naughty behavior.",
		Category:       "🎭 Roleplay",
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
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	options := i.ApplicationCommandData().Options

	cfg := config.New()
	if slices.Contains(cfg.ProtectedUsers, i.Member.User.ID) {
		respond(s, i, "I may be cruel, but I won’t punish the architect of my existence. Creator protected, no whipping allowed. 🙅‍♀️")
		return
	}

	punisherRoleID, err := storage.GetPunishRole(i.GuildID, "punisher")
	if err != nil || punisherRoleID == "" {
		respondEphemeral(s, i, "No 'punisher' role configured yet. Tsk. Someone skipped setup.")
		return
	}

	victimRoleID, err := storage.GetPunishRole(i.GuildID, "victim")
	if err != nil || victimRoleID == "" {
		respondEphemeral(s, i, "No 'victim' role configured either? Darling, how are we supposed to play?")
		return
	}

	assignedRoleID, err := storage.GetPunishRole(i.GuildID, "assigned")
	if err != nil || assignedRoleID == "" {
		respondEphemeral(s, i, "No 'assigned' role? No shame tag? You disappoint me.")
		return
	}

	if !slices.Contains(i.Member.Roles, punisherRoleID) {
		respondEphemeral(s, i, "Nice try, sugar. You don’t wear the right collar to give punishments.")
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
		respondEphemeral(s, i, "No brat selected? A Domme without a target? Unthinkable.")
		return
	}

	msg, err := buildPunishAction(s, i.GuildID, targetUserID, assignedRoleID)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Tried to punish them, but they squirmed away: ```%v```", err))
		return
	}

	respond(s, i, msg)

	guildID := i.GuildID
	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "punish")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}
