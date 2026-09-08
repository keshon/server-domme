module github.com/keshon/server-domme

go 1.26

require (
	github.com/keshon/datastore v1.3.0
	github.com/rs/zerolog v1.35.1
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require (
	github.com/cloudflare/circl v1.6.5 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)

require (
	github.com/bwmarrin/discordgo v0.29.0
	github.com/caarlos0/env/v11 v11.4.1
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/joho/godotenv v1.5.1
	github.com/keshon/buildinfo v0.1.0
	github.com/keshon/command v0.1.0
)

// discordgo is pinned to a local fork, shared byte-identical with melodix.
// Upstream deadlocks when an Op 7 or Op 9 arrives as the second packet of
// Open(): both handlers call CloseWithCode, which takes the session lock Open
// already holds. See docs/architecture.md.
replace github.com/bwmarrin/discordgo => ./pkg/discordgo-fork-dev
