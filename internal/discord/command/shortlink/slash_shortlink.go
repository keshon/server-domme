package shortlink

import (
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

type ShortlinkCommand struct{}

func (c *ShortlinkCommand) Name() string             { return "shortlink" }
func (c *ShortlinkCommand) Description() string      { return "Shorten URLs and manage your links" }
func (c *ShortlinkCommand) Group() string            { return "shortlink" }
func (c *ShortlinkCommand) Category() string         { return "📢 Utilities" }
func (c *ShortlinkCommand) UserPermissions() []int64 { return []int64{} }

func (c *ShortlinkCommand) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionSubCommand,
				Name:        "create",
				Description: "Shorten a URL",
				Options: []adapter.SlashOption{
					{
						Type:        adapter.OptionString,
						Name:        "url",
						Description: "The URL to shorten",
						Required:    true,
					},
				},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        "list",
				Description: "List your shortened URLs",
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        "delete",
				Description: "Delete a specific shortened URL",
				Options: []adapter.SlashOption{
					{
						Type:        adapter.OptionString,
						Name:        "id",
						Description: "The short ID of the link to delete (e.g. abc123)",
						Required:    true,
					},
				},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        "clear",
				Description: "Clear all your shortened URLs",
				Options: []adapter.SlashOption{
					{
						Type:        adapter.OptionString,
						Name:        "confirm",
						Description: "Type 'yes' to confirm the action",
						Required:    true,
					},
				},
			},
		},
	}
}

func (c *ShortlinkCommand) Run(ctx *adapter.SlashInteractionContext) error {
	sub, ok := ctx.FirstOption()
	if !ok {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "No subcommand provided.",
			Color:       reply.EmbedColor,
		})
	}

	switch sub.Name {
	case "create":
		return c.runCreate(ctx, sub)
	case "list":
		return c.runList(ctx)
	case "delete":
		return c.runDelete(ctx, sub)
	case "clear":
		return c.runClear(ctx, sub)
	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Unknown subcommand.",
			Color:       reply.EmbedColor,
		})
	}
}

func (c *ShortlinkCommand) runCreate(
	ctx *adapter.SlashInteractionContext,
	sub adapter.SlashArgument,
) error {
	cfg := ctx.Config
	if cfg == nil {
		return ctx.RespondEphemeral(&adapter.Embed{Description: "Config not available.",
			Color: reply.EmbedColor})
	}
	urlOpt, _ := sub.Option("url")
	raw := strings.TrimSpace(urlOpt.StringValue())

	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		if looksLikeDomain(raw) {
			raw = "https://" + raw
		}
	}

	if !isValidURL(raw) {
		return ctx.RespondEphemeral(&adapter.Embed{
			Color:       reply.EmbedColor,
			Description: fmt.Sprintf("`%s` doesn't look like a valid link.\nTry something like `https://example.com`.", raw),
		})
	}

	userID := ctx.UserID()
	guildID := ctx.GuildID()

	links, _ := ctx.Storage.GetUserShortLinks(guildID, userID)
	if len(links) >= 50 {
		return ctx.RespondEphemeral(&adapter.Embed{
			Color:       reply.EmbedColor,
			Description: "You have reached the maximum number of short links (50). Use `/shortlink clear` to clear them or `/shortlink delete` to delete some.",
		})
	}

	shortID := randomID(6)
	shortURL := fmt.Sprintf("%s/%s", cfg.ShortLinkBaseURL, shortID)

	if err := ctx.Storage.AddShortLink(guildID, userID, raw, shortID); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Color:       reply.EmbedColor,
			Description: fmt.Sprintf("Failed to save short link: `%v`", err),
		})
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Color: reply.EmbedColor,
		Title: "Short Link Created",
		Description: fmt.Sprintf(
			"**Original:** %s\n**Shortened:** %s\n\n💡 You can delete this later with `/shortlink delete id:%s`",
			raw, shortURL, shortID,
		),
	})
}

func (c *ShortlinkCommand) runList(ctx *adapter.SlashInteractionContext) error {
	cfg := ctx.Config
	if cfg == nil {
		return ctx.RespondEphemeral(&adapter.Embed{Description: "Config not available.",
			Color: reply.EmbedColor})
	}
	userID := ctx.UserID()
	guildID := ctx.GuildID()

	links, err := ctx.Storage.GetUserShortLinks(guildID, userID)
	if err != nil || len(links) == 0 {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "You don't have any shortened links yet.",
			Color:       reply.EmbedColor,
		})
	}

	// Reverse order: newest first
	for i, j := 0, len(links)-1; i < j; i, j = i+1, j-1 {
		links[i], links[j] = links[j], links[i]
	}

	shortDomain := cfg.ShortLinkBaseURL
	var embeds []*adapter.Embed
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
			embeds = append(embeds, &adapter.Embed{Description: current.String(),
				Color: reply.EmbedColor})
			current.Reset()
			current.WriteString("**(continued)**\n\n")
		}

		current.WriteString(line)
	}

	embeds = append(embeds, &adapter.Embed{Description: current.String(),
		Color: reply.EmbedColor})

	for i, embed := range embeds {
		if i == 0 {
			if err := ctx.RespondEphemeral(embed); err != nil {
				return fmt.Errorf("shortlink: failed to respond to interaction: %w", err)
			}
		} else {
			if err := ctx.FollowupEphemeral(embed); err != nil {
				return fmt.Errorf("shortlink: failed to send followup (%d/%d): %w", i+1, len(embeds), err)
			}
		}
	}

	return nil
}

func (c *ShortlinkCommand) runDelete(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	idOpt, _ := sub.Option("id")
	shortID := idOpt.StringValue()
	userID := ctx.UserID()
	guildID := ctx.GuildID()

	st := ctx.Storage
	err := st.DeleteShortLink(guildID, userID, shortID)
	if err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Color:       reply.EmbedColor,
			Description: fmt.Sprintf("Failed to delete short link: `%v`", err),
		})
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Color:       reply.EmbedColor,
		Description: fmt.Sprintf("Short link **%s** has been deleted.", shortID),
	})
}

func (c *ShortlinkCommand) runClear(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	confirmOpt, _ := sub.Option("confirm")
	if strings.ToLower(confirmOpt.StringValue()) != "yes" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Action not confirmed. Please type 'yes' to proceed.",
			Color:       reply.EmbedColor,
		})
	}
	userID := ctx.UserID()
	guildID := ctx.GuildID()

	if err := ctx.Storage.ClearUserShortLinks(guildID, userID); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Color:       reply.EmbedColor,
			Description: fmt.Sprintf("Failed to clear links: `%v`", err),
		})
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Color:       reply.EmbedColor,
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
