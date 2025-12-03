package discord

type SystemEventType string

const (
	SystemEventRefreshCommands SystemEventType = "refresh_commands"
)

type SystemEvent struct {
	Type    SystemEventType
	GuildID string
	Target  string
}

var systemEventBus = make(chan SystemEvent, 16)

func PublishSystemEvent(evt SystemEvent) {
	select {
	case systemEventBus <- evt:
	default:
		// avoid blocking; drop if too many events
	}
}

func SystemEvents() <-chan SystemEvent {
	return systemEventBus
}
