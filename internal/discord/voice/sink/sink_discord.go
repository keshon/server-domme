package sink

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/bwmarrin/discordgo"
	"github.com/godeps/opus"
	"github.com/keshon/melodix/pkg/music/stream"
)

// DiscordSink implements musicsink.AudioSink by encoding PCM to opus and sending to a voice connection.
type DiscordSink struct {
	vc *discordgo.VoiceConnection
}

func (d *DiscordSink) Stream(src io.ReadCloser, stop <-chan struct{}) error {
	return streamToDiscord(src, stop, d.vc)
}

// streamToDiscord streams PCM audio from a reader to a Discord voice connection.
// Uses stream package constants (SampleRate, Channels, FrameSize) for format.
// The caller owns the read closer and must close it when done; streamToDiscord does not close it.
func streamToDiscord(src io.ReadCloser, stop <-chan struct{}, vc *discordgo.VoiceConnection) error {
	encoder, err := opus.NewEncoder(stream.SampleRate, stream.Channels, opus.AppAudio)
	if err != nil {
		return fmt.Errorf("encoder error: %w", err)
	}
	defer encoder.Reset()

	pcmBuf := make([]byte, stream.FrameSize*stream.Channels*2)
	intBuf := make([]int16, stream.FrameSize*stream.Channels)
	opusBuf := make([]byte, 4096)

	for {
		select {
		case <-stop:
			return stream.ErrPlaybackStopped
		default:
		}

		_, err := io.ReadFull(src, pcmBuf)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		for i := range intBuf {
			intBuf[i] = int16(binary.LittleEndian.Uint16(pcmBuf[i*2 : i*2+2]))
		}

		n, err := encoder.Encode(intBuf, opusBuf)
		if err != nil {
			return fmt.Errorf("encode error: %w", err)
		}

		packet := append([]byte(nil), opusBuf[:n]...)
		select {
		case <-stop:
			return stream.ErrPlaybackStopped
		default:
			if !safeOpusSend(vc, packet) {
				return stream.ErrVoiceTransport
			}
		}
	}
}

func safeOpusSend(vc *discordgo.VoiceConnection, packet []byte) (sent bool) {
	defer func() { _ = recover() }()
	vc.OpusSend <- packet
	return true
}

