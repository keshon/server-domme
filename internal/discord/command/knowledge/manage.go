package knowledge

import (
	"fmt"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

// ManageOptions returns the settings options for the knowledge base.
// Wired as the knowledge group under /settings (admin only).
func ManageOptions() []adapter.SlashOption {
	return []adapter.SlashOption{
		{
			Type:        adapter.OptionSubCommand,
			Name:        "add",
			Description: "Add or replace a knowledge note",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionString,
					Name:        "title",
					Description: "Short title, used as the note key",
					Required:    true,
				},
				{
					Type:        adapter.OptionString,
					Name:        "body",
					Description: "The note text",
					Required:    true,
				},
				{
					Type:        adapter.OptionString,
					Name:        "source",
					Description: "Where this came from, optional",
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "remove",
			Description: "Remove a knowledge note",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionString,
					Name:        "title",
					Description: "Title of the note to remove",
					Required:    true,
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "list",
			Description: "List knowledge note titles",
		},
	}
}

// RunManage handles the knowledge settings subcommands.
func RunManage(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	switch sub.Name {
	case "add":
		titleOpt, _ := sub.Option("title")
		bodyOpt, _ := sub.Option("body")
		sourceOpt, _ := sub.Option("source")
		doc, err := ctx.Storage.UpsertKnowledgeDoc(ctx.GuildID(), titleOpt.StringValue(), bodyOpt.StringValue(), sourceOpt.StringValue())
		if err != nil {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: fmt.Sprintf("Failed to save note: `%v`", err),
				Color:       reply.EmbedColor,
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Saved note `%s`.", doc.Title),
			Color:       reply.EmbedColor,
		})
	case "remove":
		titleOpt, _ := sub.Option("title")
		if err := ctx.Storage.DeleteKnowledgeDoc(ctx.GuildID(), titleOpt.StringValue()); err != nil {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: fmt.Sprintf("Failed to remove note: `%v`", err),
				Color:       reply.EmbedColor,
			})
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Note removed.",
			Color:       reply.EmbedColor,
		})
	case "list":
		docs := ctx.Storage.ListKnowledgeDocs(ctx.GuildID())
		if len(docs) == 0 {
			return ctx.RespondEphemeral(&adapter.Embed{
				Description: "No knowledge notes yet. Add one with `/settings knowledge add`.",
				Color:       reply.EmbedColor,
			})
		}
		out := ""
		for i, d := range docs {
			out += fmt.Sprintf("%d. %s\n", i+1, d.Title)
			if len(out) > 3500 {
				out += "…"
				break
			}
		}
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: out,
			Color:       reply.EmbedColor,
		})
	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
			Color:       reply.EmbedColor,
		})
	}
}
