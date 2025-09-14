package core

type SystemEventType int

const (
	SystemEventRefreshCommands SystemEventType = iota
)

type SystemEvent struct {
	Type    SystemEventType
	GuildID string
	Target  string // "all" or a specific command name
}

var systemEvents = make(chan SystemEvent, 32)

func PublishSystemEvent(ev SystemEvent) {
	select {
	case systemEvents <- ev:
	default:
		// fallback: channel full, drop event
		// (or log if you want)]
	}
}

func SystemEvents() <-chan SystemEvent {
	return systemEvents
}
