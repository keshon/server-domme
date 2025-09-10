// /internal/core/stream/discord.go
package stream

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/bwmarrin/discordgo"
	"layeh.com/gopus"
)

func StreamToDiscord(stream io.ReadCloser, stop <-chan struct{}, vc *discordgo.VoiceConnection) error {
	encoder, err := gopus.NewEncoder(sampleRate, channels, gopus.Audio)
	if err != nil {
		return fmt.Errorf("encoder error: %w", err)
	}

	defer stream.Close()

	pcmBuf := make([]byte, frameSize*channels*2)
	intBuf := make([]int16, frameSize*channels)

	for {
		select {
		case <-stop:
			return nil
		default:
			_, err := io.ReadFull(stream, pcmBuf)
			if err != nil {
				return fmt.Errorf("read error: %w", err)
			}

			for i := range intBuf {
				intBuf[i] = int16(binary.LittleEndian.Uint16(pcmBuf[i*2 : i*2+2]))
			}

			opus, err := encoder.Encode(intBuf, frameSize, len(pcmBuf))
			if err != nil {
				return fmt.Errorf("encode error: %w", err)
			}

			vc.OpusSend <- opus
		}
	}
}
