// /commands/nuke-stop.go
package commands

func init() {
	Register(&Command{
		Sort:           1000,
		Name:           "nuke-stop",
		Description:    "Stop the active message deletion process",
		Category:       "Moderation",
		DCSlashHandler: stopSlashHandler,
	})
}

func stopSlashHandler(ctx *SlashContext) {
	s, i, storage := ctx.Session, ctx.InteractionCreate, ctx.Storage
	channelID, guildID := i.ChannelID, i.GuildID

	stopNuke(channelID)

	nukeJob, err := storage.GetNukeJob(guildID, channelID)
	if err != nil {
		respondEphemeral(s, i, "No nuke active here. Drama averted.")
		return
	}
	storage.ClearNukeJob(nukeJob.GuildID, nukeJob.ChannelID)

	respond(s, i, "Nuke cancelled. How merciful of you.")

}
