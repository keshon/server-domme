package media

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/command"
	"github.com/keshon/server-domme/internal/discord/discordreply"
	mediastore "github.com/keshon/server-domme/internal/media"
	"github.com/keshon/server-domme/internal/storage"
)

type ManageMediaCommand struct{}

func (c *ManageMediaCommand) Name() string        { return "manage-media" }
func (c *ManageMediaCommand) Description() string { return "Media settings" }
func (c *ManageMediaCommand) Group() string       { return "media" }
func (c *ManageMediaCommand) Category() string    { return "⚙️ Settings" }
func (c *ManageMediaCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func (c *ManageMediaCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "add-category",
				Description: "Add a new media category",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "name",
						Description: "Category name",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list-categories",
				Description: "List all existing media categories",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "remove-category",
				Description: "Remove a media category",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "name",
						Description: "Category name to remove",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "set-default-category",
				Description: "Set a default media category for this server",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "name",
						Description: "Category name to set as default",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "reset-default-category",
				Description: "Reset the default media category to none",
			},
		},
	}
}

func (c *ManageMediaCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*command.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	st := context.Storage
	store := context.MediaStore
	guildID := e.GuildID

	if err := discordreply.RespondDeferredEphemeral(s, e); err != nil {
		log.Printf("[ERROR] Failed to defer interaction: %v", err)
		return err
	}

	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	sub := data.Options[0]
	switch sub.Name {
	case "add-category":
		return c.runAddCategory(s, e, *st, store, guildID, sub)
	case "list-categories":
		return c.runListCategories(s, e, *st, store, guildID)
	case "remove-category":
		return c.runRemoveCategory(s, e, *st, guildID, sub)
	case "set-default-category":
		return c.runSetDefaultCategory(s, e, *st, guildID, sub)
	case "reset-default-category":
		return c.runResetDefaultCategory(s, e, *st, guildID)
	default:
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
		})
	}
}

func (c *ManageMediaCommand) runAddCategory(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, store mediastore.Store, guildID string, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	name := sanitizeCategory(sub.Options[0].StringValue())
	if name == "" {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Category name cannot be empty.",
		})
	}

	existing, err := st.GetMediaCategories(guildID)
	if err != nil {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to load categories: %v", err),
		})
	}

	for _, c := range existing {
		if c == name {
			return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Category `%s` already exists.", name),
			})
		}
	}

	if err := st.CreateMediaCategory(guildID, name); err != nil {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to create category: %v", err),
		})
	}

	if store != nil {
		if err := store.Mkdir(context.Background(), guildID, name); err != nil {
			log.Printf("[WARN] Failed to create remote category dir %s/%s: %v", guildID, name, err)
		}
	}

	return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Added new category: `%s`", name),
	})
}

func (c *ManageMediaCommand) runListCategories(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, store mediastore.Store, guildID string) error {
	cats, err := st.GetMediaCategories(guildID)
	if err != nil {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to load categories: %v", err),
		})
	}

	if len(cats) == 0 {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No categories found.",
		})
	}

	list := strings.Builder{}
	for i, cat := range cats {
		count := "?"
		if store != nil {
			if files, err := store.List(context.Background(), guildID, cat); err == nil {
				count = fmt.Sprintf("%d", len(files))
			}
		}
		list.WriteString(fmt.Sprintf("%d. `%s` — %s file(s)\n", i+1, cat, count))
	}

	return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Title:       "📂 Media Categories",
		Description: list.String(),
	})
}

func (c *ManageMediaCommand) runRemoveCategory(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, guildID string, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	name := sub.Options[0].StringValue()

	existing, err := st.GetMediaCategories(guildID)
	if err != nil {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to load categories: %v", err),
		})
	}

	found := false
	for _, c := range existing {
		if c == name {
			found = true
			break
		}
	}

	if !found {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Category `%s` not found.", name),
		})
	}

	if err := st.RemoveMediaCategory(guildID, name); err != nil {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to remove category: %v", err),
		})
	}

	return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Removed category: `%s`\n-# Files on remote storage were not deleted.", name),
	})
}

func (c *ManageMediaCommand) runSetDefaultCategory(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, guildID string, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	name := sub.Options[0].StringValue()

	existing, err := st.GetMediaCategories(guildID)
	if err != nil {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to load categories: %v", err),
		})
	}

	found := false
	for _, c := range existing {
		if c == name {
			found = true
			break
		}
	}

	if !found {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Category `%s` not found.", name),
		})
	}

	if err := st.SetMediaDefault(guildID, name); err != nil {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to set default category: %v", err),
		})
	}

	return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Set default category to: `%s`", name),
	})
}

func (c *ManageMediaCommand) runResetDefaultCategory(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, guildID string) error {
	if err := st.ResetMediaDefault(guildID); err != nil {
		return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to reset default category: %v", err),
		})
	}
	return discordreply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: "Default category reset.",
	})
}
