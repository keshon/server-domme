package media

import (
	"fmt"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

// ManageSettingsOptions returns the settings options for media management.
// Wired as the media group under /settings. The settings invocation is
// deferred before delegating here, so every answer is a followup.
func ManageSettingsOptions() []adapter.SlashOption {
	return []adapter.SlashOption{
		{
			Type:        adapter.OptionSubCommand,
			Name:        "category-add",
			Description: "Add a media category",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionString,
					Name:        "name",
					Description: "Category name",
					Required:    true,
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "category-list",
			Description: "List media categories",
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "category-remove",
			Description: "Remove a media category",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionString,
					Name:        "name",
					Description: "Category name to remove",
					Required:    true,
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "default-set",
			Description: "Set the default media category",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionString,
					Name:        "name",
					Description: "Category name to set as default",
					Required:    true,
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "default-show",
			Description: "Show the default media category",
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "default-reset",
			Description: "Clear the default media category",
		},
	}
}

// RunManageSettings handles the media settings subcommands. The caller defers
// first: category storage reads are quick, but deferring keeps the three-second
// acknowledgement from depending on it.
func RunManageSettings(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	switch sub.Name {
	case "category-add":
		return runAddCategory(ctx, sub)
	case "category-list":
		return runListCategories(ctx)
	case "category-remove":
		return runRemoveCategory(ctx, sub)
	case "default-set":
		return runSetDefaultCategory(ctx, sub)
	case "default-show":
		return runShowDefaultCategory(ctx)
	case "default-reset":
		return runResetDefaultCategory(ctx)
	default:
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
			Color:       reply.EmbedColor,
		})
	}
}

func subName(sub adapter.SlashArgument) string {
	opt, _ := sub.Option("name")
	return opt.StringValue()
}

func runAddCategory(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	name := subName(sub)

	existing, err := ctx.Storage.GetMediaCategories(ctx.GuildID())
	if err != nil {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to load categories: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	for _, c := range existing {
		if c == name {
			return ctx.FollowupEphemeral(&adapter.Embed{
				Description: fmt.Sprintf("Category `%s` already exists.", name),
				Color:       reply.EmbedColor,
			})
		}
	}

	if err := ctx.Storage.CreateMediaCategory(ctx.GuildID(), name); err != nil {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to create category: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	return ctx.FollowupEphemeral(&adapter.Embed{
		Description: fmt.Sprintf("Added new category: `%s`", name),
		Color:       reply.EmbedColor,
	})
}

func runListCategories(ctx *adapter.SlashInteractionContext) error {
	cats, err := ctx.Storage.GetMediaCategories(ctx.GuildID())
	if err != nil {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to load categories: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	if len(cats) == 0 {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: "No categories found.",
			Color:       reply.EmbedColor,
		})
	}

	list := ""
	for i, cat := range cats {
		list += fmt.Sprintf("%d. %s\n", i+1, cat)
	}

	return ctx.FollowupEphemeral(&adapter.Embed{
		Title:       "📂 Media Categories",
		Description: list,
		Color:       reply.EmbedColor,
	})
}

func runRemoveCategory(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	name := subName(sub)

	existing, err := ctx.Storage.GetMediaCategories(ctx.GuildID())
	if err != nil {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to load categories: `%v`", err),
			Color:       reply.EmbedColor,
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
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Category `%s` not found.", name),
			Color:       reply.EmbedColor,
		})
	}

	if err := ctx.Storage.RemoveMediaCategory(ctx.GuildID(), name); err != nil {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to remove category: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	return ctx.FollowupEphemeral(&adapter.Embed{
		Description: fmt.Sprintf("Removed category: `%s`", name),
		Color:       reply.EmbedColor,
	})
}

func runSetDefaultCategory(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	name := subName(sub)

	existing, err := ctx.Storage.GetMediaCategories(ctx.GuildID())
	if err != nil {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to load categories: `%v`", err),
			Color:       reply.EmbedColor,
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
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Category `%s` not found.", name),
			Color:       reply.EmbedColor,
		})
	}

	if err := ctx.Storage.SetMediaDefault(ctx.GuildID(), name); err != nil {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to set default category: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	return ctx.FollowupEphemeral(&adapter.Embed{
		Description: fmt.Sprintf("Set default category to: `%s`", name),
		Color:       reply.EmbedColor,
	})
}

func runShowDefaultCategory(ctx *adapter.SlashInteractionContext) error {
	name, err := ctx.Storage.GetMediaDefault(ctx.GuildID())
	if err != nil || name == "" {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: "No default media category set.",
			Color:       reply.EmbedColor,
		})
	}
	return ctx.FollowupEphemeral(&adapter.Embed{
		Description: fmt.Sprintf("Default media category is `%s`.", name),
		Color:       reply.EmbedColor,
	})
}

func runResetDefaultCategory(ctx *adapter.SlashInteractionContext) error {
	if err := ctx.Storage.ResetMediaDefault(ctx.GuildID()); err != nil {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to reset default category: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}
	return ctx.FollowupEphemeral(&adapter.Embed{
		Description: "Default category reset.",
		Color:       reply.EmbedColor,
	})
}
