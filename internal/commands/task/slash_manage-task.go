package task

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"server-domme/internal/core"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type ManageTaskCommand struct{}

func (c *ManageTaskCommand) Name() string        { return "manage-task" }
func (c *ManageTaskCommand) Description() string { return "Manage task-related settings" }
func (c *ManageTaskCommand) Group() string       { return "task" }
func (c *ManageTaskCommand) Category() string    { return "⚙️ Settings" }
func (c *ManageTaskCommand) UserPermissions() []int64 {
	return []int64{
		discordgo.PermissionAdministrator,
	}
}

func (c *ManageTaskCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "set-role",
				Description: "Set or update a Tasker role",
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
				Name:        "list-role",
				Description: "List all task-related roles",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "reset-role",
				Description: "Reset the Tasker role configuration",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "upload-tasks",
				Description: "Upload a new task list for this server",
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
				Name:        "download-tasks",
				Description: "Download the current task list for this server",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "reset-tasks",
				Description: "Reset the task list to default for this server",
			},
		},
	}
}

func (c *ManageTaskCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	st := context.Storage
	data := e.ApplicationCommandData()

	if len(data.Options) == 0 {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	opt := data.Options[0]
	return c.runManage(s, e, st, opt)
}

func (c *ManageTaskCommand) runManage(s *discordgo.Session, e *discordgo.InteractionCreate, storage *storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {

	switch sub.Name {

	case "set-role":
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
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Missing required options.",
			})
		}

		if err := storage.SetTaskRole(e.GuildID, roleID); err != nil {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to set Tasker role: %v", err),
			})
		}

		roleName := roleID
		if rName, err := getRoleNameByID(s, e.GuildID, roleID); err == nil {
			roleName = rName
		}

		core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Tasker role set to **%s**.", roleName),
		})
		return nil

	case "list-role":
		roleID, err := storage.GetTaskRole(e.GuildID)
		if err != nil || roleID == "" {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "No Tasker role set.",
			})
		}

		roleName := roleID
		if rName, err := getRoleNameByID(s, e.GuildID, roleID); err == nil {
			roleName = rName
		}

		core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Tasker role set to **%s**.", roleName),
		})
		return nil

	case "reset-role":
		if err := storage.SetTaskRole(e.GuildID, ""); err != nil {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to reset Tasker role: %v", err),
			})
		}

		core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Tasker role reset.",
		})
		return nil

	case "download-tasks":
		path := filepath.Join("data", fmt.Sprintf("%s_task.list.json", e.GuildID))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "No tasks file found for this server.",
			})
		}

		if err := core.RespondDeferredEphemeral(s, e); err != nil {
			return fmt.Errorf("failed to defer interaction: %w", err)
		}

		file, err := os.Open(path)
		if err != nil {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to open tasks file: %v", err),
			})
		}
		defer file.Close()

		_, err = s.FollowupMessageCreate(e.Interaction, true, &discordgo.WebhookParams{
			Content: "Here’s the task list for this server:",
			Files: []*discordgo.File{
				{
					Name:   filepath.Base(path),
					Reader: file,
				},
			},
		})
		if err != nil {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to send tasks file: %v", err),
			})
		}
		return nil

	case "upload-tasks":
		if len(sub.Options) == 0 {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "No file uploaded.",
			})
		}

		attachmentOption := sub.Options[0]
		attachmentID, ok := attachmentOption.Value.(string)
		if !ok {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Failed to retrieve attachment ID.",
			})
		}

		attachment, exists := e.ApplicationCommandData().Resolved.Attachments[attachmentID]
		if !exists {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Failed to get the uploaded file.",
			})
		}

		resp, err := http.Get(attachment.URL)
		if err != nil {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Failed to download the uploaded file.",
			})
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil || len(body) == 0 {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Failed to read the uploaded file or file is empty.",
			})
		}

		var tasks []map[string]interface{}
		if err := json.Unmarshal(body, &tasks); err != nil {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Invalid JSON file.",
			})
		}

		if err := os.MkdirAll("data", 0755); err != nil {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to create data directory: %v", err),
			})
		}

		path := filepath.Join("data", fmt.Sprintf("%s_task.list.json", e.GuildID))
		if err := os.WriteFile(path, body, 0644); err != nil {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to write tasks file: %v", err),
			})
		}

		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Tasks have been uploaded.\nSaved as `%s`", filepath.Base(path)),
		})

	case "reset-tasks":
		path := filepath.Join("data", fmt.Sprintf("%s_task.list.json", e.GuildID))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "No tasks file found for this server.",
			})
		}

		if err := os.Remove(path); err != nil {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to remove tasks file: %v", err),
			})
		}

		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Tasks have been reset. Use `/manage-task upload-tasks` to upload new tasks.",
		})

	default:
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Invalid subcommand.",
		})
	}
}

func getRoleNameByID(s *discordgo.Session, guildID, roleID string) (string, error) {
	guild, err := s.State.Guild(guildID)
	if err != nil || guild == nil {
		guild, err = s.Guild(guildID)
		if err != nil {
			return "", fmt.Errorf("failed to fetch guild: %w", err)
		}
	}
	for _, role := range guild.Roles {
		if role.ID == roleID {
			return role.Name, nil
		}
	}
	return "", fmt.Errorf("role ID %s not found in guild %s", roleID, guildID)
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&ManageTaskCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
