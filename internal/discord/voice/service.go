package voice

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/melodix/pkg/music/player"
	"github.com/keshon/melodix/pkg/music/resolve"
	"github.com/keshon/melodix/pkg/music/sources"

	"server-domme/internal/config"
	"server-domme/internal/discord/voice/sink"
)

type guildMusicStatus struct {
	ChannelID string
	MessageID string
}

// VoiceState holds minimal voice channel state for a user.
type VoiceState struct {
	ChannelID string
	UserID    string
}

// Service provides voice/music for a Discord bot: players, resolver, and guild music status.
type Service struct {
	getSession func() *discordgo.Session
	cfg        *config.Config

	mu            sync.RWMutex
	players       map[string]*player.Player
	sinkProviders map[string]*sink.DiscordSinkProvider
	resolver      *resolve.Resolver

	guildMusicStatus   map[string]guildMusicStatus
	guildMusicStatusMu sync.RWMutex
}

func New(getSession func() *discordgo.Session, cfg *config.Config) *Service {
	return &Service{
		getSession:       getSession,
		cfg:              cfg,
		players:          make(map[string]*player.Player),
		sinkProviders:    make(map[string]*sink.DiscordSinkProvider),
		guildMusicStatus: make(map[string]guildMusicStatus),
	}
}

func (s *Service) GetOrCreatePlayer(guildID string) *player.Player {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sinkProviders == nil {
		s.sinkProviders = make(map[string]*sink.DiscordSinkProvider)
	}

	if p, ok := s.players[guildID]; ok {
		p.SetGuildID(guildID)
		return p
	}

	if s.resolver == nil {
		s.resolver = resolve.New()
	}

	provider, ok := s.sinkProviders[guildID]
	if !ok {
		voiceDelay := time.Duration(s.cfg.VoiceReadyDelayMs) * time.Millisecond
		provider = sink.NewDiscordSinkProvider(func() *discordgo.Session {
			return s.getSession()
		}, guildID, voiceDelay)
		s.sinkProviders[guildID] = provider
	}

	p := player.New(provider, s.resolver)
	p.SetGuildID(guildID)
	s.players[guildID] = p
	return p
}

func (s *Service) Resolve(guildID, input, source, parser string) ([]sources.TrackInfo, error) {
	s.mu.Lock()
	if s.resolver == nil {
		s.resolver = resolve.New()
	}
	r := s.resolver
	s.mu.Unlock()
	return r.Resolve(input, source, parser)
}

func (s *Service) FindUserVoiceState(guildID, userID string) (*VoiceState, error) {
	session := s.getSession()
	if session == nil {
		return nil, fmt.Errorf("discord session not available")
	}
	guild, err := session.State.Guild(guildID)
	if err != nil {
		return nil, fmt.Errorf("error retrieving guild: %w", err)
	}
	for _, vs := range guild.VoiceStates {
		if vs.UserID == userID {
			return &VoiceState{ChannelID: vs.ChannelID, UserID: vs.UserID}, nil
		}
	}
	return nil, fmt.Errorf("user not in any voice channel")
}

func (s *Service) UpdateGuildMusicStatus(session *discordgo.Session, i *discordgo.InteractionCreate, guildID string, embed *discordgo.MessageEmbed) error {
	s.guildMusicStatusMu.RLock()
	msg, ok := s.guildMusicStatus[guildID]
	s.guildMusicStatusMu.RUnlock()

	if ok {
		_, err := session.ChannelMessageEditEmbed(msg.ChannelID, msg.MessageID, embed)
		return err
	}

	if i == nil {
		return nil
	}

	m, err := session.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
	})
	if err != nil {
		return err
	}
	if m == nil {
		return nil
	}

	s.guildMusicStatusMu.Lock()
	s.guildMusicStatus[guildID] = guildMusicStatus{ChannelID: m.ChannelID, MessageID: m.ID}
	s.guildMusicStatusMu.Unlock()
	return nil
}

func (s *Service) StopAllPlayers() {
	s.mu.Lock()
	players := make(map[string]*player.Player, len(s.players))
	for k, v := range s.players {
		players[k] = v
	}
	s.players = make(map[string]*player.Player)
	s.sinkProviders = nil
	s.resolver = nil
	s.mu.Unlock()

	for _, p := range players {
		_ = p.Stop(true)
	}
	log.Println("[INFO] All players stopped")
}

