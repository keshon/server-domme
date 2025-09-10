// /internal/core/stream/stream.go
package stream

import (
	"fmt"
	"io"
	"server-domme/internal/music/parsers"
	"server-domme/internal/music/parsers/ffmpeg"
	"server-domme/internal/music/parsers/kkdai"
	"server-domme/internal/music/parsers/ytdlp"
)

const (
	channels   = 2
	sampleRate = 48000
	frameSize  = 960 // 20ms at 48kHz
)

var StreamersRegistry = map[string]parsers.Streamer{
	"ytdlp-link":  &ytdlp.YTDLPStreamer{},
	"ytdlp-pipe":  &ytdlp.YTDLPStreamer{},
	"kkdai-link":  &kkdai.KKDAIStreamer{},
	"kkdai-pipe":  &kkdai.KKDAIStreamer{},
	"ffmpeg-link": &ffmpeg.FFMPEGStreamer{},
}

func isPipeMode(parser string) bool {
	return parser == "ytdlp-pipe" || parser == "kkdai-pipe"
}

func AutoOpenStream(track *parsers.TrackParse) (*TrackStream, func(), string, error) {
	return OpenStream(track, track.CurrentParser, 0)
}

type TrackStream struct {
	io.ReadCloser
	track  *parsers.TrackParse
	parser string
}

func (m *TrackStream) GetTrack() *parsers.TrackParse {
	return m.track
}

func (m *TrackStream) GetMode() string {
	return m.parser
}

func OpenStream(track *parsers.TrackParse, parser string, seekSec float64) (*TrackStream, func(), string, error) {
	var (
		r        io.ReadCloser
		cleanup  func()
		err      error
		usedMode string = parser
	)

	streamer, ok := StreamersRegistry[parser]
	if !ok {
		return nil, nil, parser, fmt.Errorf("streamer not found for parser: %v", parser)
	}

	if isPipeMode(parser) && streamer.SupportsPipe() {
		r, cleanup, err = streamer.GetPipeStream(track, seekSec)
	} else {
		r, cleanup, err = streamer.GetLinkStream(track, seekSec)
	}

	if err != nil {
		return nil, nil, usedMode, err
	}

	stream := &TrackStream{
		ReadCloser: r,
		track:      track,
		parser:     usedMode,
	}

	return stream, cleanup, usedMode, nil
}
