package media

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/storage"
)

// RunManageMediaSettings handles media settings subcommands.
func RunManageMediaSettings(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, guildID string, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	switch sub.Name {
	case "category-add":
		return runAddCategory(s, e, st, guildID, sub)
	case "category-list":
		return runListCategories(s, e, st, guildID)
	case "category-remove":
		return runRemoveCategory(s, e, st, guildID, sub)
	case "default-set":
		return runSetDefaultCategory(s, e, st, guildID, sub)
	case "default-show":
		return runShowDefaultCategory(s, e, st, guildID)
	case "default-reset":
		return runResetDefaultCategory(s, e, st, guildID)
	default:
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
		})
	}
}

// MediaSettingsOptions returns slash options for media settings.
func MediaSettingsOptions() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "category-add",
			Description: "Add a media category",
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
			Name:        "category-list",
			Description: "List media categories",
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "category-remove",
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
			Name:        "default-set",
			Description: "Set the default media category",
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
			Name:        "default-show",
			Description: "Show the default media category",
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "default-reset",
			Description: "Clear the default media category",
		},
	}
}

func runAddCategory(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, guildID string, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	name := sub.Options[0].StringValue()

	existing, err := st.GetMediaCategories(guildID)
	if err != nil {
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to load categories: %v", err),
		})
	}

	for _, c := range existing {
		if c == name {
			return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Category `%s` already exists.", name),
			})
		}
	}

	if err := st.CreateMediaCategory(guildID, name); err != nil {
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to create category: %v", err),
		})
	}

	return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Added new category: `%s`", name),
	})
}

func runListCategories(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, guildID string) error {
	cats, err := st.GetMediaCategories(guildID)
	if err != nil {
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to load categories: %v", err),
		})
	}

	if len(cats) == 0 {
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No categories found.",
		})
	}

	list := ""
	for i, cat := range cats {
		list += fmt.Sprintf("%d. %s\n", i+1, cat)
	}

	return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Title:       "📂 Media Categories",
		Description: list,
	})
}

func runRemoveCategory(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, guildID string, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	name := sub.Options[0].StringValue()

	existing, err := st.GetMediaCategories(guildID)
	if err != nil {
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
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
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Category `%s` not found.", name),
		})
	}

	if err := st.RemoveMediaCategory(guildID, name); err != nil {
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to remove category: %v", err),
		})
	}

	return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Removed category: `%s`", name),
	})
}

func runSetDefaultCategory(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, guildID string, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	name := sub.Options[0].StringValue()

	existing, err := st.GetMediaCategories(guildID)
	if err != nil {
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
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
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Category `%s` not found.", name),
		})
	}

	if err := st.SetMediaDefault(guildID, name); err != nil {
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to set default category: %v", err),
		})
	}

	return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Set default category to: `%s`", name),
	})
}

func runShowDefaultCategory(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, guildID string) error {
	name, err := st.GetMediaDefault(guildID)
	if err != nil || name == "" {
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No default media category set.",
		})
	}
	return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Default media category is `%s`.", name),
	})
}

func runResetDefaultCategory(s *discordgo.Session, e *discordgo.InteractionCreate, st storage.Storage, guildID string) error {
	if err := st.ResetMediaDefault(guildID); err != nil {
		return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to reset default category: %v", err),
		})
	}
	return reply.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: "Default category reset.",
	})
}
