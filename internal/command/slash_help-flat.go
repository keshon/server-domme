package command

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type HelpFlatCommand struct{}

func (c *HelpFlatCommand) Name() string        { return "help-flat" }
func (c *HelpFlatCommand) Description() string { return "Show all commands as a flat list" }
func (c *HelpFlatCommand) Aliases() []string   { return []string{} }

func (c *HelpFlatCommand) Group() string    { return "core" }
func (c *HelpFlatCommand) Category() string { return "🕯️ Information" }

func (c *HelpFlatCommand) RequireAdmin() bool { return false }
func (c *HelpFlatCommand) RequireDev() bool   { return false }

func (c *HelpFlatCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *HelpFlatCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	output := buildFlatHelpMessage(session, event)

	embed := &discordgo.MessageEmbed{
		Title:       "Flat Help",
		Description: output,
		Color:       embedColor,
	}

	err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Println("Failed to send flat help embed:", err)
		return nil
	}

	logErr := logCommand(session, storage, event.GuildID, event.ChannelID, event.Member.User.ID, event.Member.User.Username, "help-flat")
	if logErr != nil {
		log.Println("Failed to log help-flat command:", logErr)
	}
	return nil
}

func buildFlatHelpMessage(s *discordgo.Session, i *discordgo.InteractionCreate) string {
	userID := i.Member.User.ID
	all := All()

	var available []Command

	for _, cmd := range all {
		if cmd.RequireAdmin() && !isAdministrator(s, i.GuildID, i.Member) {
			continue
		}
		if cmd.RequireDev() && !isDeveloper(userID) {
			continue
		}
		available = append(available, cmd)
	}

	sort.Slice(available, func(i, j int) bool {
		return available[i].Name() < available[j].Name()
	})

	var sb strings.Builder
	for _, cmd := range available {
		sb.WriteString(fmt.Sprintf("`%s` - %s\n", cmd.Name(), cmd.Description()))
	}
	return sb.String()
}

func init() {
	Register(
		WithGroupAccessCheck()(
			WithGuildOnly(
				&HelpFlatCommand{},
			),
		),
	)
}
