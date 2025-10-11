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

func (c *TaskCommand) runManage(s *discordgo.Session, e *discordgo.InteractionCreate, storage *storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	if !core.IsAdministrator(s, e.Member) {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "You must be an admin to use this command.",
		})
	}

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
			return core.RespondEphemeral(s, e, "Missing `role` parameter.")
		}

		if err := storage.SetTaskRole(e.GuildID, roleID); err != nil {
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed saving Tasker role: `%s`", err))
		}

		roleName := roleID
		if rName, err := getRoleNameByID(s, e.GuildID, roleID); err == nil {
			roleName = rName
		}

		core.RespondEphemeral(s, e, fmt.Sprintf("✅ Tasker role set to **%s**.", roleName))
		return nil

	case "list-role":
		roleID, err := storage.GetTaskRole(e.GuildID)
		if err != nil || roleID == "" {
			return core.RespondEphemeral(s, e, "No Tasker role configured yet.")
		}

		roleName := roleID
		if rName, err := getRoleNameByID(s, e.GuildID, roleID); err == nil {
			roleName = rName
		}

		core.RespondEphemeral(s, e, fmt.Sprintf("Current Tasker role: **%s**", roleName))
		return nil

	case "reset-role":
		if err := storage.SetTaskRole(e.GuildID, ""); err != nil {
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed resetting Tasker role: %v", err))
		}

		core.RespondEphemeral(s, e, "Tasker role has been reset.")
		return nil

	case "download-tasks":
		path := filepath.Join("data", fmt.Sprintf("%s_task.list.json", e.GuildID))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return core.RespondEphemeral(s, e, "No tasks file found for this server.")
		}

		if err := core.RespondDeferredEphemeral(s, e); err != nil {
			return fmt.Errorf("failed to defer interaction: %w", err)
		}

		file, err := os.Open(path)
		if err != nil {
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to open tasks file: %v", err))
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
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to send tasks file: %v", err))
		}
		return nil

	case "upload-tasks":
		if len(sub.Options) == 0 {
			return core.RespondEphemeral(s, e, "No file uploaded.")
		}

		attachmentOption := sub.Options[0]
		attachmentID, ok := attachmentOption.Value.(string)
		if !ok {
			return core.RespondEphemeral(s, e, "Failed to retrieve attachment ID.")
		}

		attachment, exists := e.ApplicationCommandData().Resolved.Attachments[attachmentID]
		if !exists {
			return core.RespondEphemeral(s, e, "Failed to get the uploaded file.")
		}

		resp, err := http.Get(attachment.URL)
		if err != nil {
			return core.RespondEphemeral(s, e, "Failed to download the uploaded file.")
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil || len(body) == 0 {
			return core.RespondEphemeral(s, e, "Failed to read the uploaded file or file is empty.")
		}

		var tasks []map[string]interface{}
		if err := json.Unmarshal(body, &tasks); err != nil {
			return core.RespondEphemeral(s, e, "Invalid JSON file.")
		}

		if err := os.MkdirAll("data", 0755); err != nil {
			return core.RespondEphemeral(s, e, "Failed to create data directory.")
		}

		path := filepath.Join("data", fmt.Sprintf("%s_task.list.json", e.GuildID))
		if err := os.WriteFile(path, body, 0644); err != nil {
			return core.RespondEphemeral(s, e, "Failed to save uploaded tasks file.")
		}

		return core.RespondEphemeral(s, e, fmt.Sprintf("Successfully uploaded tasks file: `%s`. Use `/task manage download-tasks` to download it.", filepath.Base(path)))

	case "reset-tasks":
		path := filepath.Join("data", fmt.Sprintf("%s_task.list.json", e.GuildID))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return core.RespondEphemeral(s, e, "No tasks file to reset — already clean.")
		}

		if err := os.Remove(path); err != nil {
			return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to reset tasks: %v", err))
		}

		return core.RespondEphemeral(s, e, "Tasks have been reset. Use `/task manage upload-tasks` to upload new tasks.")

	default:
		return core.RespondEphemeral(s, e, fmt.Sprintf("Unknown manage roles subcommand: %s", sub.Name))
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
