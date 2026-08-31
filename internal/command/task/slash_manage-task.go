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

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/storage"
)

// RunManageTaskSettings handles task settings subcommands.
func RunManageTaskSettings(s *discordgo.Session, e *discordgo.InteractionCreate, storage *storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	switch sub.Name {

	case "role-set":
		var roleID string
		for _, opt := range sub.Options {
			if opt.Name == "role" {
				role := opt.RoleValue(s, e.GuildID)
				if role != nil {
					roleID = role.ID
				}
			}
		}

		if roleID == "" {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Missing required options.",
			})
		}

		if err := storage.SetTaskRole(e.GuildID, roleID); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to set Tasker role: %v", err),
			})
		}

		roleName := roleID
		if rName, err := getRoleNameByID(s, e.GuildID, roleID); err == nil {
			roleName = rName
		}

		reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Tasker role set to **%s**.", roleName),
		})
		return nil

	case "role-show":
		roleID, err := storage.GetTaskRole(e.GuildID)
		if err != nil || roleID == "" {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "No Tasker role set.",
			})
		}

		roleName := roleID
		if rName, err := getRoleNameByID(s, e.GuildID, roleID); err == nil {
			roleName = rName
		}

		reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Tasker role set to **%s**.", roleName),
		})
		return nil

	case "role-reset":
		if err := storage.SetTaskRole(e.GuildID, ""); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to reset Tasker role: %v", err),
			})
		}

		reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Tasker role reset.",
		})
		return nil

	case "tasks-download":
		path := filepath.Join("data", fmt.Sprintf("%s_task.list.json", e.GuildID))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "No tasks file found for this server.",
			})
		}

		if err := reply.RespondDeferredEphemeral(s, e); err != nil {
			return fmt.Errorf("task: defer interaction: %w", err)
		}

		file, err := os.Open(path)
		if err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to open tasks file: %v", err),
			})
		}
		defer file.Close()

		_, err = s.FollowupMessageCreate(e.Interaction, true, &discordgo.WebhookParams{
			Content: "Here's the task list for this server:",
			Files: []*discordgo.File{
				{
					Name:   filepath.Base(path),
					Reader: file,
				},
			},
		})
		if err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to send tasks file: %v", err),
			})
		}
		return nil

	case "tasks-upload":
		if len(sub.Options) == 0 {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "No file uploaded.",
			})
		}

		attachmentOption := sub.Options[0]
		attachmentID, ok := attachmentOption.Value.(string)
		if !ok {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Failed to retrieve attachment ID.",
			})
		}

		attachment, exists := e.ApplicationCommandData().Resolved.Attachments[attachmentID]
		if !exists {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Failed to get the uploaded file.",
			})
		}

		resp, err := http.Get(attachment.URL)
		if err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Failed to download the uploaded file.",
			})
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil || len(body) == 0 {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Failed to read the uploaded file or file is empty.",
			})
		}

		var tasks []map[string]interface{}
		if err := json.Unmarshal(body, &tasks); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Invalid JSON file.",
			})
		}

		if err := os.MkdirAll("data", 0755); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to create data directory: %v", err),
			})
		}

		path := filepath.Join("data", fmt.Sprintf("%s_task.list.json", e.GuildID))
		if err := os.WriteFile(path, body, 0644); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to write tasks file: %v", err),
			})
		}

		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Tasks have been uploaded.\nSaved as `%s`", filepath.Base(path)),
		})

	case "tasks-reset":
		path := filepath.Join("data", fmt.Sprintf("%s_task.list.json", e.GuildID))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "No tasks file found for this server.",
			})
		}

		if err := os.Remove(path); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to remove tasks file: %v", err),
			})
		}

		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Tasks have been reset. Use `/settings task tasks-upload` to upload new tasks.",
		})

	case "cooldown-set":
		var durationRaw string
		for _, opt := range sub.Options {
			if opt.Name == "duration" {
				durationRaw = opt.StringValue()
			}
		}

		if durationRaw == "" {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Missing required options.",
			})
		}

		if err := storage.SetTaskCooldownDuration(e.GuildID, durationRaw); err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Invalid duration: %v\nUse `30m`, `3h`, `1d`, etc.", err),
			})
		}

		duration, _ := storage.GetTaskCooldownDuration(e.GuildID)
		reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Task cooldown set to **%s**.", humanDuration(duration)),
		})
		return nil

	case "cooldown-show":
		duration, err := storage.GetTaskCooldownDuration(e.GuildID)
		if err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to read cooldown setting: %v", err),
			})
		}

		isDefault, _ := storage.IsTaskCooldownDurationDefault(e.GuildID)
		guildLine := fmt.Sprintf("**Guild cooldown:** %s", humanDuration(duration))
		if isDefault {
			guildLine += " (default)"
		}

		active, err := storage.ListActiveTaskCooldowns(e.GuildID)
		if err != nil {
			return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to list cooldowns: %v", err),
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

		reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: guildLine + "\n\n" + activeSection,
		})
		return nil

	default:
		return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Invalid subcommand.",
		})
	}
}

// TaskSettingsOptions returns slash options for task settings.
func TaskSettingsOptions() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "role-set",
			Description: "Configure the Tasker role",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionRole,
					Name:        "role",
					Description: "Select the role allowed to get tasks",
					Required:    true,
				},
			},
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "role-show",
			Description: "Show the configured Tasker role",
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "role-reset",
			Description: "Reset the Tasker role configuration",
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "tasks-upload",
			Description: "Upload a task list",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionAttachment,
					Name:        "file",
					Description: "JSON file (.json) containing the task list",
					Required:    true,
				},
			},
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "tasks-download",
			Description: "Download the current task list",
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "tasks-reset",
			Description: "Reset tasks to defaults",
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "cooldown-set",
			Description: "Set task cooldown duration",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "duration",
					Description: "Cooldown after completing/failing a task (e.g. 30m, 3h, 1d)",
					Required:    true,
				},
			},
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "cooldown-show",
			Description: "Show cooldown settings and active cooldowns",
		},
	}
}

func getRoleNameByID(s *discordgo.Session, guildID, roleID string) (string, error) {
	guild, err := s.State.Guild(guildID)
	if err != nil || guild == nil {
		guild, err = s.Guild(guildID)
		if err != nil {
			return "", fmt.Errorf("task: fetch guild: %w", err)
		}
	}
	for _, role := range guild.Roles {
		if role.ID == roleID {
			return role.Name, nil
		}
	}
	return "", fmt.Errorf("task: role ID %s not found in guild %s", roleID, guildID)
}
