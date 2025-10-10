package commands

import (
	_ "server-domme/internal/commands/announce"
	_ "server-domme/internal/commands/ask"
	_ "server-domme/internal/commands/chat"
	_ "server-domme/internal/commands/confess"
	_ "server-domme/internal/commands/core"
	_ "server-domme/internal/commands/music"
	_ "server-domme/internal/commands/punish"
	_ "server-domme/internal/commands/purge"
	_ "server-domme/internal/commands/roll"
	_ "server-domme/internal/commands/task"
	_ "server-domme/internal/commands/translate"
)

// import all commands to trigger init
