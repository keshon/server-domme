package link

import (
	"fmt"
	"math/rand"
	"strings"

	"server-domme/internal/bot"
	"server-domme/internal/config"
	"server-domme/internal/middleware"
	"server-domme/internal/registry"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type ShortenCommand struct{}

func (c *ShortenCommand) Name() string        { return "shorten" }
func (c *ShortenCommand) Description() string { return "Shorten URLs and manage your links" }
func (c *ShortenCommand) Group() string       { return "shorten" }
func (c *ShortenCommand) Category() string    { return "📢 Utilities" }

func (c *ShortenCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *ShortenCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "create",
				Description: "Shorten a URL",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "url",
						Description: "The URL to shorten",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "List your shortened URLs",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "delete",
				Description: "Delete a specific shortened URL",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "id",
						Description: "The short ID of the link to delete (e.g. abc123)",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "clear",
				Description: "Clear all your shortened URLs",
			},
		},
	}
}

func (c *ShortenCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*registry.SlashInteractionContext)
	if !ok {
		return nil
	}
	s := context.Session
	e := context.Event
	st := context.Storage
	data := e.ApplicationCommandData()

	if len(data.Options) == 0 {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	opt := data.Options[0]
	switch opt.Name {
	case "create":
		return c.runCreate(s, e, st, opt)
	case "list":
		return c.runList(s, e, st)
	case "delete":
		return c.runDelete(s, e, st, opt)
	case "clear":
		return c.runClear(s, e, st)
	default:
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Unknown subcommand.",
		})
	}
}

func (c *ShortenCommand) runCreate(s *discordgo.Session, e *discordgo.InteractionCreate, st *storage.Storage, opt *discordgo.ApplicationCommandInteractionDataOption) error {
	config := config.New()

	rawURL := opt.Options[0].StringValue()

	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	shortID := randomID(6)
	shortDomain := config.ShortLinkBaseURL
	shortURL := fmt.Sprintf("%s/%s", shortDomain, shortID)

	userID := e.Member.User.ID
	guildID := e.GuildID

	if err := st.AddShortLink(guildID, userID, rawURL, shortID); err != nil {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to save short link: %v", err),
		})
	}

	return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Title:       "Short Link Created",
		Description: fmt.Sprintf("**Original:** %s\n**Shortened:** [%s](%s)", rawURL, shortURL, shortURL),
	})
}

func (c *ShortenCommand) runList(s *discordgo.Session, e *discordgo.InteractionCreate, st *storage.Storage) error {
	config := config.New()

	userID := e.Member.User.ID
	guildID := e.GuildID

	links, err := st.GetUserShortLinks(guildID, userID)
	if err != nil || len(links) == 0 {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "You don’t have any shortened links yet.",
		})
	}

	shortDomain := config.ShortLinkBaseURL
	var out strings.Builder
	for _, link := range links {
		out.WriteString(fmt.Sprintf("• [%s/%s](%s/%s) → %s\n", shortDomain, link.ShortID, shortDomain, link.ShortID, link.Original))
	}

	return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Title:       "🔗 Your Shortened Links",
		Description: out.String(),
	})
}

func (c *ShortenCommand) runDelete(s *discordgo.Session, e *discordgo.InteractionCreate, st *storage.Storage, opt *discordgo.ApplicationCommandInteractionDataOption) error {
	shortID := opt.Options[0].StringValue()
	userID := e.Member.User.ID
	guildID := e.GuildID

	err := st.DeleteShortLink(guildID, userID, shortID)
	if err != nil {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to delete short link: %v", err),
		})
	}

	return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Short link **%s** has been deleted.", shortID),
	})
}

func (c *ShortenCommand) runClear(s *discordgo.Session, e *discordgo.InteractionCreate, st *storage.Storage) error {
	userID := e.Member.User.ID
	guildID := e.GuildID

	if err := st.ClearUserShortLinks(guildID, userID); err != nil {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to clear links: %v", err),
		})
	}

	return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: "All your shortened links have been cleared.",
	})
}

func randomID(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func init() {
	registry.RegisterCommand(
		middleware.ApplyMiddlewares(
			&ShortenCommand{},
			middleware.WithGroupAccessCheck(),
			middleware.WithGuildOnly(),
			middleware.WithUserPermissionCheck(),
			middleware.WithCommandLogger(),
		),
	)
}
