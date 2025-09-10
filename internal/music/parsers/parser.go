package parsers

import (
	"io"
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

type Streamer interface {
	GetLinkStream(track *TrackParse, seekSec float64) (io.ReadCloser, func(), error)
	GetPipeStream(track *TrackParse, seekSec float64) (io.ReadCloser, func(), error)
	SupportsPipe() bool
}
