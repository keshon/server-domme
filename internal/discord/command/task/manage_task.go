package task

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

// ManageSettingsOptions returns the settings options for task management.
// Wired as the task group under /settings.
func ManageSettingsOptions() []adapter.SlashOption {
	return []adapter.SlashOption{
		{
			Type:        adapter.OptionSubCommand,
			Name:        "role-set",
			Description: "Configure the Tasker role",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionRole,
					Name:        "role",
					Description: "Select the role allowed to get tasks",
					Required:    true,
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "role-show",
			Description: "Show the configured Tasker role",
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "role-reset",
			Description: "Reset the Tasker role configuration",
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "tasks-upload",
			Description: "Upload a task list",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionAttachment,
					Name:        "file",
					Description: "JSON file (.json) containing the task list",
					Required:    true,
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "tasks-download",
			Description: "Download the current task list",
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "tasks-reset",
			Description: "Reset tasks to defaults",
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "cooldown-set",
			Description: "Set task cooldown duration",
			Options: []adapter.SlashOption{
				{
					Type:        adapter.OptionString,
					Name:        "duration",
					Description: "Cooldown after completing/failing a task (e.g. 30m, 3h, 1d)",
					Required:    true,
				},
			},
		},
		{
			Type:        adapter.OptionSubCommand,
			Name:        "cooldown-show",
			Description: "Show cooldown settings and active cooldowns",
		},
	}
}

// RunManageSettings handles the task settings subcommands.
func RunManageSettings(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	switch sub.Name {
	case "role-set":
		return runRoleSet(ctx, sub)
	case "role-show":
		return runRoleShow(ctx)
	case "role-reset":
		return runRoleReset(ctx)
	case "tasks-download":
		return runTasksDownload(ctx)
	case "tasks-upload":
		return runTasksUpload(ctx, sub)
	case "tasks-reset":
		return runTasksReset(ctx)
	case "cooldown-set":
		return runCooldownSet(ctx, sub)
	case "cooldown-show":
		return runCooldownShow(ctx)
	default:
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Invalid subcommand.",
			Color:       reply.EmbedColor,
		})
	}
}

func runRoleSet(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	roleOpt, _ := sub.Option("role")
	roleID := roleOpt.StringValue()

	if roleID == "" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Missing required options.",
			Color:       reply.EmbedColor,
		})
	}

	if err := ctx.Storage.SetTaskRole(ctx.GuildID(), roleID); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to set Tasker role: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	roleName := roleID
	if rName, err := ctx.API.RoleName(ctx.GuildID(), roleID); err == nil {
		roleName = rName
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Description: fmt.Sprintf("Tasker role set to **%s**.", roleName),
		Color:       reply.EmbedColor,
	})
}

func runRoleShow(ctx *adapter.SlashInteractionContext) error {
	roleID, err := ctx.Storage.GetTaskRole(ctx.GuildID())
	if err != nil || roleID == "" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "No Tasker role set.",
			Color:       reply.EmbedColor,
		})
	}

	roleName := roleID
	if rName, err := ctx.API.RoleName(ctx.GuildID(), roleID); err == nil {
		roleName = rName
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Description: fmt.Sprintf("Tasker role set to **%s**.", roleName),
		Color:       reply.EmbedColor,
	})
}

func runRoleReset(ctx *adapter.SlashInteractionContext) error {
	if err := ctx.Storage.SetTaskRole(ctx.GuildID(), ""); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to reset Tasker role: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Description: "Tasker role reset.",
		Color:       reply.EmbedColor,
	})
}

func runTasksDownload(ctx *adapter.SlashInteractionContext) error {
	path := filepath.Join("data", fmt.Sprintf("%s_task.list.json", ctx.GuildID()))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "No tasks file found for this server.",
			Color:       reply.EmbedColor,
		})
	}

	if err := ctx.DeferEphemeral(); err != nil {
		return fmt.Errorf("task: defer interaction: %w", err)
	}

	file, err := os.Open(path)
	if err != nil {
		return ctx.FollowupEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to open tasks file: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}
	defer file.Close()

	return ctx.FollowupWith(adapter.Reply{
		Text:      "Here's the task list for this server:",
		File:      file,
		FileName:  filepath.Base(path),
		Ephemeral: true,
	})
}

func runTasksUpload(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	fileOpt, _ := sub.Option("file")
	attachment, ok := ctx.Attachments[fileOpt.StringValue()]
	if !ok {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Failed to get the uploaded file.",
			Color:       reply.EmbedColor,
		})
	}

	resp, err := http.Get(attachment.URL)
	if err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Failed to download the uploaded file.",
			Color:       reply.EmbedColor,
		})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || len(body) == 0 {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Failed to read the uploaded file or file is empty.",
			Color:       reply.EmbedColor,
		})
	}

	var tasks []map[string]interface{}
	if err := json.Unmarshal(body, &tasks); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Invalid JSON file.",
			Color:       reply.EmbedColor,
		})
	}

	if err := os.MkdirAll("data", 0755); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to create data directory: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	path := filepath.Join("data", fmt.Sprintf("%s_task.list.json", ctx.GuildID()))
	if err := os.WriteFile(path, body, 0644); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to write tasks file: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Description: fmt.Sprintf("Tasks have been uploaded.\nSaved as `%s`", filepath.Base(path)),
		Color:       reply.EmbedColor,
	})
}

func runTasksReset(ctx *adapter.SlashInteractionContext) error {
	path := filepath.Join("data", fmt.Sprintf("%s_task.list.json", ctx.GuildID()))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "No tasks file found for this server.",
			Color:       reply.EmbedColor,
		})
	}

	if err := os.Remove(path); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to remove tasks file: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Description: "Tasks have been reset. Use `/settings task tasks-upload` to upload new tasks.",
		Color:       reply.EmbedColor,
	})
}

func runCooldownSet(ctx *adapter.SlashInteractionContext, sub adapter.SlashArgument) error {
	durationOpt, _ := sub.Option("duration")
	durationRaw := durationOpt.StringValue()

	if durationRaw == "" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Missing required options.",
			Color:       reply.EmbedColor,
		})
	}

	if err := ctx.Storage.SetTaskCooldownDuration(ctx.GuildID(), durationRaw); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Invalid duration: `%v`\nUse `30m`, `3h`, `1d`, etc.", err),
			Color:       reply.EmbedColor,
		})
	}

	duration, _ := ctx.Storage.GetTaskCooldownDuration(ctx.GuildID())
	return ctx.RespondEphemeral(&adapter.Embed{
		Description: fmt.Sprintf("Task cooldown set to **%s**.", humanDuration(duration)),
		Color:       reply.EmbedColor,
	})
}

func runCooldownShow(ctx *adapter.SlashInteractionContext) error {
	duration, err := ctx.Storage.GetTaskCooldownDuration(ctx.GuildID())
	if err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to read cooldown setting: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	isDefault, _ := ctx.Storage.IsTaskCooldownDurationDefault(ctx.GuildID())
	guildLine := fmt.Sprintf("**Guild cooldown:** %s", humanDuration(duration))
	if isDefault {
		guildLine += " (default)"
	}

	active, err := ctx.Storage.ListActiveTaskCooldowns(ctx.GuildID())
	if err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to list cooldowns: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	var cooldownLines []string
	now := time.Now()
	for userID, expiry := range active {
		cooldownLines = append(cooldownLines, fmt.Sprintf("• <@%s> — expires in %s", userID, humanDuration(expiry.Sub(now))))
	}

	activeSection := "**Active cooldowns:**\nNone"
	if len(cooldownLines) > 0 {
		activeSection = "**Active cooldowns:**\n" + strings.Join(cooldownLines, "\n")
	}

	return ctx.RespondEphemeral(&adapter.Embed{
		Description: guildLine + "\n\n" + activeSection,
		Color:       reply.EmbedColor,
	})
}
