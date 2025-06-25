// /discordtypes/discordtypes.go
package discordtypes

type CommandMode int

const (
	ModePrefix CommandMode = iota
	ModeSlash
)

func ParseCommandMode(input string) CommandMode {
	switch input {
	case "slash":
		return ModeSlash
	default:
		return ModePrefix
	}
}

type GuildSettings struct {
	Prefix string
	Mode   CommandMode
}
