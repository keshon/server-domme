package sources

const (
	SourceAuto       = "auto"
	SourceYouTube    = "youtube"
	SourceRadio      = "radio"
	SourceSoundCloud = "soundcloud"
)

type TrackInfo struct {
	URL              string
	Title            string
	SourceName       string
	AvailableParsers []string
}

type Source interface {
	// Match checks if this source can handle the given input
	Match(input string) bool

	// Resolve turns an input into one or more playable tracks
	Resolve(input string, selectedParser string) ([]TrackInfo, error)

	// Type returns the string identifier ("youtube", "radio", etc.)
	SourceName() string

	// AvailableParsers returns the list of parsers supported by this source
	AvailableParsers() []string
}
