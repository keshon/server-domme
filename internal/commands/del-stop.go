package commands

import "log"

func init() {
	Register(&Command{
		Sort:           230,
		Name:           "del-stop",
		Category:       "🧹 Channel Cleanup",
		Description:    "Halt ongoing deletions in a channel",
		AdminOnly:      true,
		DCSlashHandler: stopSlashHandler,
	})
}

func stopSlashHandler(ctx *SlashContext) {
	if !RequireGuild(ctx) {
		return
	}
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	channelID, guildID := i.ChannelID, i.GuildID

	if !isAdministrator(s, guildID, i.Member) {
		respondEphemeral(s, i, "You must be a server administrator to use this command.")
		return
	}

	stopDeletion(channelID)

	_, err := storage.GetDeletionJob(guildID, channelID)
	if err == nil {
		_ = storage.ClearDeletionJob(guildID, channelID)
		respondEphemeral(s, i, "Message deletion job stopped.")
	} else {
		respondEphemeral(s, i, "There was no message deletion job, but I stopped any deletions anyway. You're welcome.")
	}

	userID := i.Member.User.ID
	username := i.Member.User.Username
	err = logCommand(s, ctx.Storage, guildID, i.ChannelID, userID, username, "del-stop")
	if err != nil {
		log.Println("Failed to log command:", err)
	}
}
