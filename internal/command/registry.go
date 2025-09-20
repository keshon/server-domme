package command

import (
	"server-domme/internal/command/announce"
	"server-domme/internal/core"
)

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&announce.AnnounceCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}
