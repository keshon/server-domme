package chat

import (
	"math"
	"sort"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/mind"
)

// Things done to her, and reactions to what she says. See
// docs/persona-v3.md, B4.

// giftWindow is how far back gifts from one person are counted: the fifth
// coffee in an hour does nothing.
const giftWindow = time.Hour

// energyEvent is the day count of moments whose appraisal set energy: if it
// creeps up, the battery is turning into a mood. See the spec's risks.
const energyEvent = "energy set"

// applyEnergy gives her body what the appraisal said a moment did to her
// energy. What a gift means is the model's; the code keeps the diminishing
// returns: the k-th lift from the same person within the hour counts
// 0.5^(k-1) of itself.
func (s *Service) applyEnergy(guildID, userID string, energy float64, now time.Time) {
	if s.body == nil || energy == 0 {
		return
	}
	applied := energy
	if energy > 0 && userID != "" {
		s.giftMu.Lock()
		recent := s.gifts[userID][:0]
		for _, at := range s.gifts[userID] {
			if now.Sub(at) < giftWindow {
				recent = append(recent, at)
			}
		}
		applied *= math.Pow(0.5, float64(len(recent)))
		s.gifts[userID] = append(recent, now)
		s.giftMu.Unlock()
	}
	s.body.Drain(-applied)
	if err := s.store.CountMindEvent(guildID, s.day(), energyEvent); err != nil {
		s.log.Debug().Err(err).Str("guild_id", guildID).Msg("chat_count_failed")
	}
	s.log.Info().
		Str("guild_id", guildID).
		Float64("energy", energy).
		Float64("applied", applied).
		Float64("battery", s.battery()).
		Msg("chat_energy_applied")
}

// reactionTally is one emoji on her messages in a channel since she last
// spoke there.
type reactionTally struct {
	emoji string
	users []string
}

// noteReaction counts a reaction to one of her messages, for the next scene
// in that channel. No model call per emoji: it reaches her as a fact.
func (s *Service) noteReaction(r *discordgo.MessageReaction) {
	if r.Emoji.Name == "" {
		return
	}
	hers := false
	for _, t := range s.conv.Recent(r.ChannelID) {
		if t.FromBot && t.MessageID == r.MessageID {
			hers = true
			break
		}
	}
	if !hers {
		return
	}
	s.reactMu.Lock()
	defer s.reactMu.Unlock()
	tallies := s.reactions[r.ChannelID]
	for _, t := range tallies {
		if t.emoji == r.Emoji.Name {
			t.users = append(t.users, r.UserID)
			return
		}
	}
	s.reactions[r.ChannelID] = append(tallies, &reactionTally{emoji: r.Emoji.Name, users: []string{r.UserID}})
}

// reactionsIn is what people put on her messages in a channel since she last
// spoke there, by the names the conversation shows her.
func (s *Service) reactionsIn(channelID string, turns []mind.Turn) []mind.Reaction {
	s.reactMu.Lock()
	tallies := s.reactions[channelID]
	s.reactMu.Unlock()
	if len(tallies) == 0 {
		return nil
	}
	names := map[string]string{}
	for _, t := range turns {
		if !t.FromBot && t.Username != "" {
			names[t.UserID] = t.Username
		}
	}
	var out []mind.Reaction
	for _, t := range tallies {
		r := mind.Reaction{Emoji: t.emoji, Count: len(t.users)}
		seen := map[string]bool{}
		for _, u := range t.users {
			name := names[u]
			if name == "" {
				name = "someone"
			}
			if !seen[name] {
				seen[name] = true
				r.Names = append(r.Names, name)
			}
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// clearReactions starts a channel's tally again: she has spoken since.
func (s *Service) clearReactions(channelID string) {
	s.reactMu.Lock()
	defer s.reactMu.Unlock()
	delete(s.reactions, channelID)
}
