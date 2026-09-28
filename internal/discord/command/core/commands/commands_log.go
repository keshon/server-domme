package commands

import (
	"strings"

	"github.com/keshon/server-domme/internal/discord/adapter"
)

// RunLog shows recent command usage for the guild.
func RunLog(ctx *adapter.SlashInteractionContext) error {
	records, err := ctx.Storage.CommandHistory(ctx.GuildID())
	if err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "Failed to fetch command logs: " + err.Error(),
		})
	}
	if len(records) == 0 {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "No command logs found.",
		})
	}

	var builder strings.Builder
	builder.WriteString("Datetime           \tUsername       \tChannel     \tCommand\n")

	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]

		line := r.Datetime.Format("2006-01-02 15:04:05") + "\t" +
			r.Username + "\t#" + r.ChannelName + "\t/" + r.Command + "\n"

		if builder.Len()+len(line) > maxContentLength {
			break
		}
		builder.WriteString(line)
	}

	return ctx.RespondWith(adapter.Reply{
		Text:      codeLeftBlockWrapper + "\n" + builder.String() + codeRightBlockWrapper,
		Ephemeral: true,
	})
}
