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

type Output int

const (
	OutputDiscord Output = iota
	OutputSpeaker
)

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

var (
	ErrNoTrackPlaying  = errors.New("no track is currently playing")
	ErrNoTracksInQueue = errors.New("no tracks in queue")
)

type Player struct {
	mu           sync.Mutex
	playing      bool
	currTrack    *parsers.TrackParse
	queue        []parsers.TrackParse
	history      []parsers.TrackParse
	output       Output
	ListenerOnce sync.Once

	resolver *source_resolver.SourceResolver
	store    *storage.Storage

	dg *discordgo.Session

	guildID   string
	channelID string
	vc        *discordgo.VoiceConnection

	stopOnce     sync.Once
	stopPlayback chan struct{}
	playbackDone chan struct{}
	PlayerStatus chan PlayerStatus
}

// New creates a new Player instance
func New(dg *discordgo.Session, guildID string, store *storage.Storage, resolver *source_resolver.SourceResolver) *Player {
	return &Player{
		dg:           dg,
		guildID:      guildID,
		store:        store,
		resolver:     resolver,
		queue:        make([]parsers.TrackParse, 0),
		history:      make([]parsers.TrackParse, 0),
		stopPlayback: make(chan struct{}),
		playbackDone: make(chan struct{}),
		PlayerStatus: make(chan PlayerStatus, 10), // buffered to reduce drops
	}
}

// Enqueue adds tracks to the queue
func (p *Player) Enqueue(input string, source string, parser string) error {
	log.Printf("[Player] Enqueue called | input=%q source=%q parser=%q", input, source, parser)
	tracksInfo, err := p.resolver.Resolve(input, source, parser)
	if err != nil {
		log.Printf("[Player] Failed to resolve tracks: %v", err)
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
	log.Printf("[Player] Added %d track(s) to queue | QueueLen=%d", len(tracksParse), len(p.queue))
	if p.currTrack != nil {
		p.emitStatus(StatusAdded)
	}
	return nil
}

// PlayNext stops current track (if any) and plays the next in queue// PlayNext stops current track (if any) and plays the next in queue
func (p *Player) PlayNext(channelID string) error {
	log.Printf("[Player] PlayNext called | QueueLen=%d", len(p.queue))
	for {
		p.mu.Lock()
		if len(p.queue) == 0 {
			p.mu.Unlock()
			log.Printf("[Player] Queue is empty, nothing to play")
			return ErrNoTracksInQueue
		}

		track := p.queue[0]
		p.queue = p.queue[1:]
		p.channelID = channelID
		p.mu.Unlock()

		log.Printf("[Player] Attempting to play track %q (%s)", track.Title, track.URL)

		if p.IsPlaying() {
			log.Printf("[Player] Stopping current track before playing next")
			_ = p.Stop(false)
		}

		err := p.startTrack(&track, false)
		if err != nil {
			log.Printf("[Player] Skipping track %q due to error: %v", track.Title, err)
			continue // try next track
		}

		p.mu.Lock()
		p.currTrack = &track
		p.playing = true
		p.history = append(p.history, track)
		p.mu.Unlock()

		log.Printf("[Player] Now playing track %q | QueueLen=%d", track.Title, len(p.queue))
		return nil
	}
}

// Stop safely stops current playback
func (p *Player) Stop(exitVc bool) error {
	p.mu.Lock()
	if !p.playing {
		p.mu.Unlock()
		log.Printf("[Player] Stop called but no track is playing")
		return ErrNoTrackPlaying
	}
	p.mu.Unlock()

	log.Printf("[Player] Stop called | exitVc=%v", exitVc)

	p.stopOnce.Do(func() {
		close(p.stopPlayback)
	})

	<-p.playbackDone // wait for goroutine to finish
	log.Printf("[Player] Playback goroutine finished")

	p.mu.Lock()
	p.playing = false
	p.currTrack = nil

	if exitVc {
		log.Printf("[Player] Exiting voice channel and clearing queue")
		p.queue = nil
		p.channelID = ""
		if p.vc != nil {
			p.vc.Disconnect()
			p.vc = nil
		}
	}

	p.stopPlayback = make(chan struct{})
	p.playbackDone = make(chan struct{})
	p.stopOnce = sync.Once{}
	p.emitStatus(StatusStopped)
	p.mu.Unlock()

	log.Printf("[Player] Stop finished")
	return nil
}

// Pause pauses playback
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

// Resume resumes playback
func (p *Player) Resume() error {
	p.mu.Lock()
	if p.currTrack == nil {
		p.mu.Unlock()
		return ErrNoTrackPlaying
	}
	track := p.currTrack
	p.playing = true
	p.mu.Unlock()

	// Restart playback for resume
	return p.startTrack(track, true)
}

// IsPlaying returns current playback state
func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

// CurrentTrack returns currently playing track
func (p *Player) CurrentTrack() (*parsers.TrackParse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.playing || p.currTrack == nil {
		return nil, ErrNoTrackPlaying
	}
	return p.currTrack, nil
}

// Queue returns a copy of current queue
func (p *Player) Queue() []parsers.TrackParse {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.queue)
}

// History returns a copy of played tracks
func (p *Player) History() []parsers.TrackParse {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.history)
}

// startTrack launches playback goroutine
func (p *Player) startTrack(track *parsers.TrackParse, resumed bool) error {
	log.Printf("[Player] Preparing playback for track: %q (%s) | CurrentParser=%s | QueueLen=%d",
		track.Title, track.URL, track.CurrentParser, len(p.queue))

	currStream, cleanup, usedStreamMode, err := stream.AutoOpenStream(track)
	if err != nil {
		log.Printf("[Player] Failed to open stream for track %q: %v", track.Title, err)
		p.emitStatus(StatusError)
		return fmt.Errorf("failed to create PCM stream for track: %w", err)
	}

	log.Printf("[Player] Stream opened successfully for track %q with parser %s", track.Title, usedStreamMode)
	track.CurrentParser = usedStreamMode

	if resumed {
		p.emitStatus(StatusResumed)
		log.Printf("[Player] Resuming track %q", track.Title)
	} else {
		p.emitStatus(StatusPlaying)
		log.Printf("[Player] Starting track %q", track.Title)
	}

	go func() {
		if err := p.runPlayback(currStream, cleanup); err != nil {
			log.Printf("[Player] Playback error for track %q: %v", track.Title, err)
		}
	}()

	return nil
}

// runPlayback handles actual streaming
func (p *Player) runPlayback(currStream *stream.TrackStream, cleanup func()) error {
	defer cleanup()
	defer func() { close(p.playbackDone) }()

	var err error
	log.Printf("[Player] Running playback for track: %q", currStream.GetTrack().Title)

	switch p.output {
	case OutputSpeaker:
		log.Printf("[Player] Output mode: Speaker (not implemented)")
	default:
		vc, vErr := p.getOrCreateVoiceConnection()
		if vErr != nil {
			err = vErr
			log.Printf("[Player] Failed to get/create voice connection: %v", err)
		} else {
			p.vc = vc
			log.Printf("[Player] Streaming to Discord VC: channel=%s guild=%s", p.vc.ChannelID, p.guildID)
			stream.StreamToDiscord(currStream, p.stopPlayback, vc)
		}
	}

	p.mu.Lock()
	p.playing = false
	p.currTrack = nil
	p.mu.Unlock()

	if err != nil {
		err = fmt.Errorf("playback error: %w", err)
		p.emitStatus(StatusError)
		log.Printf("[Player] Playback finished with error: %v", err)
	} else {
		log.Printf("[Player] Playback finished successfully")
		p.emitStatus(StatusStopped)
	}

	return err
}

// getOrCreateVoiceConnection joins or reuses existing VC
func (p *Player) getOrCreateVoiceConnection() (*discordgo.VoiceConnection, error) {
	if p.channelID == "" {
		return nil, errors.New("voice channel ID is not set")
	}

	if p.vc != nil && p.vc.ChannelID == p.channelID {
		return p.vc, nil // reuse
	}

	vc, err := p.dg.ChannelVoiceJoin(p.guildID, p.channelID, false, true)
	if err != nil {
		return nil, fmt.Errorf("failed to join voice channel: %w", err)
	}
	log.Printf("[Player] Joined voice channel %s on guild %s", p.channelID, p.guildID)
	return vc, nil
}

// emitStatus safely sends player status
func (p *Player) emitStatus(status PlayerStatus) {
	select {
	case p.PlayerStatus <- status:
	default:
		log.Printf("[Player] Player status signal dropped (channel full) - %s", status)
	}
}

// SetOutput sets playback output
func (p *Player) SetOutput(mode Output) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.output = mode
}
