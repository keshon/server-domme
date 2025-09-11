package stream

import (
	"errors"
	"io"
	"log"
	"server-domme/internal/music/parsers"
)

const maxRecoveryAttempts = 3

// RecoveryStream wraps a TrackStream and attempts to auto-recover on early stream termination.
type RecoveryStream struct {
	track       *parsers.TrackParse
	parserIndex int // index in AvailableParsers
	stream      *TrackStream
	cleanup     func()
	seekSec     float64        // current playback position
	retries     map[string]int // parser => attempts
}

// NewRecoveryStream creates a new resilient wrapper for a track
func NewRecoveryStream(track *parsers.TrackParse) *RecoveryStream {
	return &RecoveryStream{
		track:   track,
		retries: make(map[string]int),
	}
}

// Open attempts to open a TrackStream for the current parser
func (rs *RecoveryStream) Open(seek float64) error {
	for i := rs.parserIndex; i < len(rs.track.SourceInfo.AvailableParsers); i++ {
		parser := rs.track.SourceInfo.AvailableParsers[i]

		if rs.retries[parser] >= maxRecoveryAttempts {
			log.Printf("[RecoveryStream] Parser %s exceeded max recovery attempts", parser)
			continue
		}

		stream, cleanup, err := openWithParser(rs.track, parser, seek)
		if err != nil {
			log.Printf("[RecoveryStream] Failed to open stream with parser %s: %v", parser, err)
			rs.retries[parser]++
			continue
		}

		rs.parserIndex = i
		rs.stream = stream
		rs.cleanup = cleanup
		rs.seekSec = seek
		log.Printf("[RecoveryStream] Successfully opened stream with parser %s at seek %.2f", parser, seek)
		return nil
	}

	return errors.New("all parsers failed or exceeded recovery attempts")
}

// Read implements io.Reader for RecoveryStream
func (rs *RecoveryStream) Read(p []byte) (int, error) {
	if rs.stream == nil {
		return 0, errors.New("stream not opened")
	}

	n, err := rs.stream.Read(p)
	if err == io.EOF && n == 0 {
		// Early EOF detected; attempt recovery
		return rs.handleRecovery(p)
	}

	// Normal read
	rs.seekSec += float64(n) / (SampleRate * Channels * 2) // approximate playback seconds
	return n, err
}

// handleRecovery attempts to reopen the stream from the current seek position
func (rs *RecoveryStream) handleRecovery(p []byte) (int, error) {
	if rs.retries[rs.track.CurrentParser] >= maxRecoveryAttempts {
		log.Printf("[RecoveryStream] Max recovery attempts reached for parser %s", rs.track.CurrentParser)
		return 0, io.EOF
	}

	log.Printf("[RecoveryStream] Stream ended prematurely, attempting recovery (attempt %d) ...", rs.retries[rs.track.CurrentParser]+1)
	rs.retries[rs.track.CurrentParser]++

	// Clean up old stream
	if rs.cleanup != nil {
		rs.cleanup()
	}

	// Reopen stream
	if err := rs.Open(rs.seekSec); err != nil {
		log.Printf("[RecoveryStream] Recovery failed: %v", err)
		return 0, io.EOF
	}

	// Retry reading
	return rs.Read(p)
}

// Close closes the underlying stream
func (rs *RecoveryStream) Close() error {
	if rs.cleanup != nil {
		rs.cleanup()
	}
	if rs.stream != nil {
		return rs.stream.Close()
	}
	return nil
}

// GetTrack returns the underlying track
func (rs *RecoveryStream) GetTrack() *parsers.TrackParse {
	return rs.track
}

// GetParser returns the current parser used
func (rs *RecoveryStream) GetParser() string {
	if rs.stream != nil {
		return rs.stream.Parser
	}
	return ""
}
