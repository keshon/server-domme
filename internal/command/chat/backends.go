package chat

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/ai"
	chatsvc "github.com/keshon/server-domme/internal/chat"
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
)

// /chat backends and its options.
const (
	subBackends = "backends"
	optOrder    = "order"
	optVoice    = "voice"
	optOff      = "off"
	optOn       = "on"
	optRanking  = "ranking"
	optReset    = "reset"
	// voiceNone clears the voice order.
	voiceNone = "none"
)

// backendsOption is /chat backends: the order the backends are tried in,
// the ones her voice prefers, and which are switched off.
func backendsOption() *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionSubCommand,
		Name:        subBackends,
		Description: "Which models she thinks and speaks through (bot developer only) — or see them, left empty",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        optOrder,
				Description: "Names, comma separated, to try first, in order; the rest follow",
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        optVoice,
				Description: `Names her voice prefers, in order, comma separated; "none" to use the full order`,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        optOff,
				Description: "A backend to switch off",
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        optOn,
				Description: "A backend to switch back on",
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        optRanking,
				Description: "priority: in the order given · score: by how they have been doing",
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "priority — in the order given", Value: string(ai.ModePriority)},
					{Name: "score — by how they have been doing", Value: string(ai.ModeScore)},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionBoolean,
				Name:        optReset,
				Description: "Forget these changes and go back to the deployment's settings",
			},
		},
	}
}

// runBackends shows or changes the backends. Developer only: the backends
// are shared by every server the bot is in, so no one server's administrator
// gets to choose them for the others.
func (c *ChatCommand) runBackends(context *cmdadapter.SlashInteractionContext, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	s, e := context.Session, context.Event

	if !config.IsDeveloper(context.Config, interactionUserID(e)) {
		msg := "Only the bot's developer can change which models she runs on: they are shared by every server the bot is in."
		if context.Config == nil || context.Config.DeveloperID == "" {
			msg += " No developer is configured — set `DEVELOPER_ID` in the deployment."
		}
		return respond(s, e, msg)
	}

	opts := map[string]*discordgo.ApplicationCommandInteractionDataOption{}
	for _, o := range sub.Options {
		opts[o.Name] = o
	}

	if o, ok := opts[optReset]; ok && o.BoolValue() {
		if err := c.Service.ResetBackends(); err != nil {
			return respond(s, e, backendError(err))
		}
		return respond(s, e, "Back to the deployment's settings.\n\n"+c.backendsReport())
	}

	if len(opts) > 0 {
		err := c.Service.ArrangeBackends(func(p *ai.Pool) error {
			if o, ok := opts[optRanking]; ok {
				mode, _ := ai.ParseMode(o.StringValue())
				p.SetMode(mode)
			}
			if o, ok := opts[optOrder]; ok {
				if err := p.SetOrder(names(o.StringValue())); err != nil {
					return err
				}
			}
			if o, ok := opts[optVoice]; ok {
				voice := names(o.StringValue())
				if len(voice) == 1 && strings.EqualFold(voice[0], voiceNone) {
					voice = nil
				}
				if err := p.SetVoiceOrder(voice); err != nil {
					return err
				}
			}
			if o, ok := opts[optOn]; ok {
				if err := p.SetOff(strings.TrimSpace(o.StringValue()), false); err != nil {
					return err
				}
			}
			if o, ok := opts[optOff]; ok {
				if err := p.SetOff(strings.TrimSpace(o.StringValue()), true); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return respond(s, e, backendError(err))
		}
		return respond(s, e, "Done. It holds across restarts.\n\n"+c.backendsReport())
	}

	return respond(s, e, c.backendsReport())
}

// backendsReport is the backends in the order she tries them.
func (c *ChatCommand) backendsReport() string {
	b, ok := c.Service.Backends()
	if !ok {
		return "Her backend cannot be arranged: it is not a pool."
	}
	var out strings.Builder
	fmt.Fprintf(&out, "**Backends** — ranked by %s", b.Mode)
	if b.Stored {
		out.WriteString(", as arranged here")
	} else {
		out.WriteString(", as the deployment set them")
	}
	out.WriteString("\n")
	out.WriteString(backendLines(b.Stats))
	out.WriteString("\n-# 🗣 marks her voice, in order. Thinking uses the whole list.")
	return out.String()
}

// backendLines renders backends one to a line, in the order given.
func backendLines(stats []ai.BackendStat) string {
	var b strings.Builder
	for i, st := range stats {
		fmt.Fprintf(&b, "%d. `%s`", i+1, st.Name)
		if st.Model != "" {
			fmt.Fprintf(&b, " · %s", st.Model)
		}
		if st.Voice > 0 {
			fmt.Fprintf(&b, " · 🗣 %d", st.Voice)
		}
		switch {
		case st.Off:
			b.WriteString(" · **off**")
		case st.CooledFor > 0:
			fmt.Fprintf(&b, " · resting %s", st.CooledFor)
		}
		fmt.Fprintf(&b, " — %d ok / %d failed\n", st.Successes, st.Failures)
		// Quoted only when nothing has ever worked. A backend that is
		// answering does not need its last hiccup shown; one that has never
		// answered is the entire reason anyone is looking.
		if st.Successes == 0 && st.LastError != "" {
			fmt.Fprintf(&b, "> %s\n", trimForEmbed(st.LastError))
		}
	}
	return b.String()
}

func backendError(err error) string {
	switch {
	case errors.Is(err, ai.ErrUnknownBackend):
		return "No backend by that name. `/chat backends` lists the names. (" + err.Error() + ")"
	case errors.Is(err, ai.ErrLastBackend):
		return "That is the last backend switched on — she would have nothing to speak through."
	case errors.Is(err, chatsvc.ErrNoPool):
		return "Her backend cannot be arranged: it is not a pool."
	}
	return "Could not change the backends: " + err.Error()
}

// names splits a comma-separated list of backend names.
func names(raw string) []string {
	var out []string
	for _, n := range strings.Split(raw, ",") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func interactionUserID(e *discordgo.InteractionCreate) string {
	if e.Member != nil && e.Member.User != nil {
		return e.Member.User.ID
	}
	if e.User != nil {
		return e.User.ID
	}
	return ""
}
