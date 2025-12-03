package parsers

import (
	"server-domme/internal/music/sources"
	"time"
)

type TrackParse struct {
	URL                 string
	Title               string
	Artist              string
	Duration            time.Duration
	CurrentPlayDuration time.Duration
	CurrentParser       string
	SourceInfo          sources.TrackInfo
}
