package commands

import "sync"

var (
	activeDeletions   = make(map[string]chan struct{})
	activeDeletionsMu sync.Mutex
)

func init() {
	Register(&Command{
		Sort:           1000,
		Name:           "stop-nuke",
		Description:    "Stop the active message deletion process",
		Category:       "Moderation",
		DCSlashHandler: stopSlashHandler,
	})
}

func stopSlashHandler(ctx *SlashContext) {
	channelID := ctx.InteractionCreate.ChannelID

	activeDeletionsMu.Lock()
	stopChan, ok := activeDeletions[channelID]
	if ok {
		close(stopChan)
		delete(activeDeletions, channelID)
		activeDeletionsMu.Unlock()
		respond(ctx.Session, ctx.InteractionCreate, "Deletion stopped. You spared them... for now.")
		return
	}
	activeDeletionsMu.Unlock()

	respondEphemeral(ctx.Session, ctx.InteractionCreate, "There's nothing to stop, darling. No deletion is active here.")
}
