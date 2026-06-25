module github.com/keshon/server-domme

go 1.26

require (
	github.com/keshon/datastore v0.1.1
	github.com/rs/zerolog v1.35.1
	gopkg.in/natefinch/lumberjack.v2 v2.2.1
)

require (
	github.com/cloudflare/circl v1.6.4 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
)

require (
	github.com/bwmarrin/discordgo v0.29.1-0.20251229154532-54ae40de5723
	github.com/caarlos0/env/v11 v11.4.1
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/joho/godotenv v1.5.1
	github.com/keshon/buildinfo v0.1.0
	github.com/keshon/command v0.1.0
	golang.org/x/time v0.15.0
)

replace github.com/bwmarrin/discordgo => ./pkg/discordgo-fork-dev
