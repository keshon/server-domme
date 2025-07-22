package commands

func init() {
	Register(&Command{
		Sort:           230,
		Name:           "del-stop",
		Description:    "Halt ongoing deletions — mercy granted.",
		Category:       "🧹 Channel Cleanup",
		DCSlashHandler: stopSlashHandler,
	})
}

func stopSlashHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	channelID, guildID := i.ChannelID, i.GuildID

	stopDeletion(channelID)

	_, err := storage.GetDeletionJob(guildID, channelID)
	if err == nil {
		_ = storage.ClearDeletionJob(guildID, channelID)
		respondEphemeral(s, i, "Message deletion job stopped.")
	} else {
		respondEphemeral(s, i, "There was no message deletion job, but I stopped any deletions anyway. You're welcome.")
	}
}
