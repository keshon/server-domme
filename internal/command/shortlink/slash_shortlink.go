package shortlink

import (
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"regexp"
	"strings"

	"server-domme/internal/bot"
	"server-domme/internal/command"
	"server-domme/internal/config"
	"server-domme/internal/middleware"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type ShortlinkCommand struct{}

func (c *ShortlinkCommand) Name() string             { return "shortlink" }
func (c *ShortlinkCommand) Description() string      { return "Shorten URLs and manage your links" }
func (c *ShortlinkCommand) Group() string            { return "shortlink" }
func (c *ShortlinkCommand) Category() string         { return "📢 Utilities" }
func (c *ShortlinkCommand) UserPermissions() []int64 { return []int64{} }

func (c *ShortlinkCommand) SlashDefinition() *discordgo.ApplicationCommand {
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

func (c *ShortlinkCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*command.SlashInteractionContext)
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

func (c *ShortlinkCommand) runCreate(
	s *discordgo.Session,
	e *discordgo.InteractionCreate,
	st *storage.Storage,
	opt *discordgo.ApplicationCommandInteractionDataOption,
) error {
	cfg := config.New()
	raw := strings.TrimSpace(opt.Options[0].StringValue())

	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		if looksLikeDomain(raw) {
			raw = "https://" + raw
		}
	}

	if !isValidURL(raw) {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Color:       0xFF0000,
			Description: fmt.Sprintf("`%s` doesn’t look like a valid link.\nTry something like `https://example.com`.", raw),
		})
	}

	userID := e.Member.User.ID
	guildID := e.GuildID

	links, _ := st.GetUserShortLinks(guildID, userID)
	if len(links) >= 50 {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Color:       0xFF0000,
			Description: "You have reached the maximum number of short links (50). Use `/shorten clear` to clear them or `/shorten delete` to delete some.",
		})
	}

	shortID := randomID(6)
	shortURL := fmt.Sprintf("%s/%s", cfg.ShortLinkBaseURL, shortID)

	if err := st.AddShortLink(guildID, userID, raw, shortID); err != nil {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Color:       0xFF0000,
			Description: fmt.Sprintf("Failed to save short link: %v", err),
		})
	}

	return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Color: bot.EmbedColor,
		Title: "Short Link Created",
		Description: fmt.Sprintf(
			"**Original:** %s\n**Shortened:** %s\n\n💡 You can delete this later with `/shorten delete id:%s`",
			raw, shortURL, shortID,
		),
	})
}

func (c *ShortlinkCommand) runList(s *discordgo.Session, e *discordgo.InteractionCreate, st *storage.Storage) error {
	cfg := config.New()
	userID := e.Member.User.ID
	guildID := e.GuildID

	links, err := st.GetUserShortLinks(guildID, userID)
	if err != nil || len(links) == 0 {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "You don’t have any shortened links yet.",
		})
	}

	// Reverse order: newest first
	for i, j := 0, len(links)-1; i < j; i, j = i+1, j-1 {
		links[i], links[j] = links[j], links[i]
	}

	shortDomain := cfg.ShortLinkBaseURL
	var embeds []*discordgo.MessageEmbed
	var current strings.Builder
	current.WriteString("**Your Shortened Links (newest first):**\n\n")

	for i, link := range links {
		shortenedURL := fmt.Sprintf("%s/%s", shortDomain, link.ShortID)
		displayShortened := strings.TrimPrefix(strings.TrimPrefix(shortenedURL, "https://"), "http://")

		displayOriginal := strings.TrimPrefix(strings.TrimPrefix(link.Original, "https://"), "http://")
		truncated := shortenLongURL(displayOriginal, 70)

		line := fmt.Sprintf(
			"**%d.** [%s](%s)\n[%s](%s)\n`ID:` `%s` ｜ **%d clicks**\n\n",
			i+1, displayShortened, shortenedURL, truncated, link.Original, link.ShortID, link.Clicks,
		)

		if len(current.String())+len(line) > 3800 {
			embeds = append(embeds, &discordgo.MessageEmbed{Description: current.String()})
			current.Reset()
			current.WriteString("**(continued)**\n\n")
		}

		current.WriteString(line)
	}

	embeds = append(embeds, &discordgo.MessageEmbed{Description: current.String()})

	for i, embed := range embeds {
		if i == 0 {
			_ = bot.RespondEmbedEphemeral(s, e, embed)
		} else {
			_, _ = s.FollowupMessageCreate(e.Interaction, true, &discordgo.WebhookParams{
				Embeds: []*discordgo.MessageEmbed{embed},
			})
		}
	}

	return nil
}

func (c *ShortlinkCommand) runDelete(s *discordgo.Session, e *discordgo.InteractionCreate, st *storage.Storage, opt *discordgo.ApplicationCommandInteractionDataOption) error {
	shortID := opt.Options[0].StringValue()
	userID := e.Member.User.ID
	guildID := e.GuildID

	err := st.DeleteShortLink(guildID, userID, shortID)
	if err != nil {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Color:       bot.EmbedColor,
			Description: fmt.Sprintf("Failed to delete short link: %v", err),
		})
	}

	return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Color:       bot.EmbedColor,
		Description: fmt.Sprintf("Short link **%s** has been deleted.", shortID),
	})
}

func (c *ShortlinkCommand) runClear(s *discordgo.Session, e *discordgo.InteractionCreate, st *storage.Storage) error {
	userID := e.Member.User.ID
	guildID := e.GuildID

	if err := st.ClearUserShortLinks(guildID, userID); err != nil {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Color:       bot.EmbedColor,
			Description: fmt.Sprintf("Failed to clear links: %v", err),
		})
	}

	return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Color:       bot.EmbedColor,
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

func looksLikeDomain(s string) bool {
	return strings.Contains(s, ".") && !strings.ContainsAny(s, " @")
}

func isValidURL(str string) bool {
	u, err := url.ParseRequestURI(str)
	if err != nil {
		return false
	}
	if u.Scheme == "" || u.Host == "" {
		return false
	}

	host := u.Hostname()
	if net.ParseIP(host) != nil {
		return true
	}

	re := regexp.MustCompile(`^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(host)
}

func shortenLongURL(s string, max int) string {
	if len(s) <= max {
		return s
	}
	start := s[:max/2-5]
	end := s[len(s)-max/2+5:]
	return fmt.Sprintf("%s...%s", start, end)
}

func init() {
	command.RegisterCommand(
		command.ApplyMiddlewares(
			&ShortlinkCommand{},
			middleware.WithGroupAccessCheck(),
			middleware.WithGuildOnly(),
			middleware.WithUserPermissionCheck(),
			middleware.WithCommandLogger(),
		),
	)
}
