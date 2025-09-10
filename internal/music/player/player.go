package player

import (
	"errors"
	"fmt"
	"log"
	"server-domme/internal/music/parsers"
	"server-domme/internal/music/source_resolver"
	"server-domme/internal/music/stream"
	"server-domme/internal/storage"
	"slices"
	"sync"

	"github.com/bwmarrin/discordgo"
)

// Output defines playback output
type Output int

const (
	OutputDiscord Output = iota
	OutputSpeaker
)

// PlayerStatus defines playback-related signals
type PlayerStatus string

const (
	StatusPlaying PlayerStatus = "Playing"
	StatusAdded   PlayerStatus = "Track(s) Added"
	StatusStopped PlayerStatus = "Playback Stopped"
	StatusPaused  PlayerStatus = "Playback Paused"
	StatusResumed PlayerStatus = "Playback Resumed"
	StatusError   PlayerStatus = "Error"
)

func (status PlayerStatus) StringEmoji() string {
	m := map[PlayerStatus]string{
		StatusPlaying: "▶️",
		StatusAdded:   "🎶",
		StatusStopped: "⏹",
		StatusPaused:  "⏸",
		StatusResumed: "▶️",
		StatusError:   "❌",
	}

	return m[status]
}

// Errors
var (
	ErrNoTrackPlaying  = errors.New("no track is currently playing")
	ErrNoTracksInQueue = errors.New("no tracks in queue")
)

// Player handles playback logic per guild
type Player struct {
	// Core state
	mu           sync.Mutex
	playing      bool
	currTrack    *parsers.TrackParse
	queue        []parsers.TrackParse
	history      []parsers.TrackParse
	output       Output
	ListenerOnce sync.Once

	// Dependencies
	resolver *source_resolver.SourceResolver
	store    *storage.Storage

	dg *discordgo.Session

	// Discord context
	guildID   string
	channelID string
	vc        *discordgo.VoiceConnection

	// Playback control
	stopPlayback chan struct{}
	PlayerStatus chan PlayerStatus
}

// New creates a new Player instance
func New(dg *discordgo.Session, guildID string, store *storage.Storage, sourceResolver *source_resolver.SourceResolver) *Player {
	return &Player{
		dg:           dg,
		guildID:      guildID,
		store:        store,
		resolver:     sourceResolver,
		queue:        make([]parsers.TrackParse, 0),
		history:      make([]parsers.TrackParse, 0),
		stopPlayback: make(chan struct{}),
		PlayerStatus: make(chan PlayerStatus, 5),
	}
}

// Public Methods – Queue / Playback API

func (p *Player) Enqueue(input string, source string, parser string) error {
	tracksInfo, err := p.resolver.Resolve(input, source, parser)
	if err != nil {
		p.emitStatus(StatusError)
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	tracksParse := make([]parsers.TrackParse, len(tracksInfo))
	for i, trackInfo := range tracksInfo {
		tracksParse[i] = parsers.TrackParse{
			URL:           trackInfo.URL,
			Title:         trackInfo.Title,
			CurrentParser: trackInfo.AvailableParsers[0],
			SourceInfo:    trackInfo,
		}
	}

	p.queue = append(p.queue, tracksParse...)
	if p.currTrack != nil {
		p.emitStatus(StatusAdded)
	}
	return nil
}

func (p *Player) PlayNext(channelID string) error {
	p.mu.Lock()
	if len(p.queue) == 0 {
		p.mu.Unlock()
		return ErrNoTracksInQueue
	}

	track := p.queue[0]
	p.queue = p.queue[1:]
	p.channelID = channelID
	p.currTrack = &track
	p.playing = true
	p.history = append(p.history, track)
	p.mu.Unlock()

	return p.startTrack(&track, false)
}

func (p *Player) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.playing {
		return ErrNoTrackPlaying
	}
	close(p.stopPlayback)
	p.playing = false
	p.currTrack = nil

	if p.vc != nil {
		p.vc.Disconnect()
		p.vc = nil
	}

	p.stopPlayback = make(chan struct{})
	p.emitStatus(StatusStopped)
	return nil
}

func (p *Player) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.playing {
		return ErrNoTrackPlaying
	}
	p.playing = false
	p.emitStatus(StatusPaused)
	return nil
}

func (p *Player) Resume() error {
	p.mu.Lock()
	if p.currTrack == nil {
		p.mu.Unlock()
		return ErrNoTrackPlaying
	}
	p.playing = true
	track := p.currTrack
	p.mu.Unlock()

	return p.startTrack(track, true)
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

func (p *Player) CurrentTrack() (*parsers.TrackParse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.playing || p.currTrack == nil {
		return nil, ErrNoTrackPlaying
	}
	return p.currTrack, nil
}

func (p *Player) Queue() []parsers.TrackParse {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.queue)
}

func (p *Player) History() []parsers.TrackParse {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.history)
}

// Internal Methods – Playback & Voice

// startTrack handles opening stream and starting playback goroutine
func (p *Player) startTrack(track *parsers.TrackParse, resumed bool) error {
	log.Printf("Preparing playback for track: %s (%s)", track.Title, track.URL)

	currStream, cleanup, usedStreamMode, err := stream.AutoOpenStream(track)
	if err != nil {
		p.emitStatus(StatusError)
		return fmt.Errorf("failed to create PCM stream for track: %w", err)
	}

	track.CurrentParser = usedStreamMode

	if resumed {
		p.emitStatus(StatusResumed)
	} else {
		p.emitStatus(StatusPlaying)
	}

	go func() {
		if err := p.runPlayback(currStream, cleanup); err != nil {
			log.Printf("Playback error in goroutine: %v", err)
		}
	}()

	return nil
}

// runPlayback starts playback in a separate goroutine
func (p *Player) runPlayback(currStream *stream.TrackStream, cleanup func()) error {
	defer cleanup()

	var err error
	switch p.output {
	case OutputSpeaker:
		// err = stream.StreamToSpeaker(currStream, p.stopPlayback)
	default:
		vc, vErr := p.getOrCreateVoiceConnection()
		if vErr != nil {
			err = vErr
		} else {
			p.vc = vc
			stream.StreamToDiscord(currStream, p.stopPlayback, vc)
		}
	}

	if err != nil {
		err = fmt.Errorf("playback error: %w", err)
		p.emitStatus(StatusError)
	}

	p.mu.Lock()
	p.playing = false
	p.currTrack = nil
	p.mu.Unlock()

	log.Println("Playback finished")
	p.emitStatus(StatusStopped)

	return err
}

func (p *Player) getOrCreateVoiceConnection() (*discordgo.VoiceConnection, error) {
	if p.channelID == "" {
		return nil, errors.New("voice channel ID is not set")
	}

	vc, err := p.dg.ChannelVoiceJoin(p.guildID, p.channelID, false, true)
	if err != nil {
		return nil, fmt.Errorf("failed to join voice channel: %w", err)
	}
	log.Printf("Joined voice channel %s on guild %s", p.channelID, p.guildID)
	return vc, nil
}

// Internal Method – Signal helper

func (p *Player) emitStatus(status PlayerStatus) {
	select {
	case p.PlayerStatus <- status:
	default:
		log.Printf("Player status signal dropped (channel full) - %s", status)
	}
}

func (p *Player) SetOutput(mode Output) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.output = mode
}
