// Package chat is the command surface for the conversational persona: which
// channels she watches, what she is told about the server, and how the
// backends behind her are behaving.
//
// The persona itself runs in internal/chat. This package only configures it
// and feeds it messages — see ObserveMessage.
package chat

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	chatsvc "github.com/keshon/server-domme/internal/chat"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/storage"
)

// Subcommand names. Renaming one costs every admin their muscle memory and
// forces a command re-sync, so treat them the way slash names are treated
// everywhere else here.
//
// v2 folded ten into seven: here, silence and proactive were three ways of
// saying how she behaves in a channel, and state was half of what status is
// for. See docs/persona.md.
const (
	subChannel = "channel"
	subBrief   = "brief"
	subStatus  = "status"
	subForget  = "forget"
	subRole    = "role"
	subAbout   = "about"
	subWhy     = "why"
	optMode    = "mode"
	optMessage = "message"
	optUser    = "user"
	optEnabled = "enabled"
	optRole    = "role"
	optNote    = "note"
	// optConfirm is the word an administrator has to type out. A button would
	// be one click from the same mistake, and this is not undoable.
	optConfirm    = "confirm"
	confirmPhrase = "yes"
)

// Channel modes, as /chat channel offers them. Each is a step up from the
// last: she cannot speak first where she does not read.
const (
	modeOff     = "off"
	modeAnswers = "answers"
	modeFirst   = "speaks-first"
)

// ChatCommand configures the persona and feeds her every message in the
// channels she has been let into.
type ChatCommand struct {
	// Service is nil when the bot is built without a chat backend, which is
	// the ordinary state for an operator who has not opted in. Every path
	// here has to tolerate that rather than assume it away.
	Service *chatsvc.Service
	// Unavailable explains why Service is nil when the persona was switched on
	// and failed to start. Empty means it was simply never switched on.
	//
	// It is surfaced to the administrator rather than only logged: they are
	// standing in Discord holding the answer to "why isn't this working", and
	// the container logs are somewhere else entirely.
	Unavailable string
}

func (c *ChatCommand) Name() string        { return "chat" }
func (c *ChatCommand) Description() string { return "Let the resident persona into a channel" }
func (c *ChatCommand) Group() string       { return "chat" }
func (c *ChatCommand) Category() string    { return "💬 Chat" }

func (c *ChatCommand) UserPermissions() []int64 {
	// Opting a channel in sends everything said there to a third-party relay.
	// That is an administrator's call, not a member's.
	return []int64{discordgo.PermissionAdministrator}
}

// ObserveReaction implements cmdadapter.ReactionObserver.
func (c *ChatCommand) ObserveReaction(ctx *cmdadapter.MessageReactionContext) {
	if c.Service == nil || ctx.Event == nil || ctx.Event.MessageReaction == nil {
		return
	}
	c.Service.ObserveReaction(ctx.Event.MessageReaction)
}

// ObserveMessage implements cmdadapter.MessageObserver.
func (c *ChatCommand) ObserveMessage(ctx *cmdadapter.MessageContext) {
	if c.Service == nil {
		return
	}
	c.Service.Observe(ctx.Session, ctx.Event)
}

func (c *ChatCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subChannel,
				Description: "How she behaves in this channel — or see it, left empty",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        optMode,
						Description: "off: not reading · answers: reads and answers · speaks-first: may also start things",
						Required:    false,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{Name: "off — she does not read this channel", Value: modeOff},
							{Name: "answers — she reads and answers when she wants to", Value: modeAnswers},
							{Name: "speaks first — she may also start things here", Value: modeFirst},
						},
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subBrief,
				Description: "Tell her what this server is, in a sentence",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "text",
						Description: "Leave empty to show what she has been told",
						Required:    false,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subStatus,
				Description: "How she is here — mood, the people, what she means to do — and how the backends are",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subRole,
				Description: "What a role means to her — how she treats anyone wearing it",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionRole,
						Name:        optRole,
						Description: "The role to set. Leave the note out to see what it is now",
						Required:    true,
					},
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        optNote,
						Description: `Said about them verbatim, e.g. "a submissive here, speak to them as one". Empty clears it`,
						Required:    false,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subWhy,
				Description: "Why she did what she did about a message — the latest here, or one you name",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        optMessage,
						Description: "A message link or id — theirs or her reply. Empty for the latest",
						Required:    false,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subAbout,
				Description: "What she knows and thinks about someone",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionUser,
						Name:        optUser,
						Description: "Who to look up",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        subForget,
				Description: "Wipe everything she remembers about this server",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        optConfirm,
						Description: `Type "yes" to confirm`,
						Required:    true,
					},
				},
			},
			backendsOption(),
		},
	}
}

func (c *ChatCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*cmdadapter.SlashInteractionContext)
	if !ok {
		return nil
	}

	s, e := context.Session, context.Event

	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		return respond(s, e, "Pick something: `channel`, `status`, `why`, `about`, `brief`, `role`, `forget` or `backends`.")
	}
	sub := data.Options[0]

	if c.Service == nil {
		return respond(s, e, unavailableMessage(c.Unavailable))
	}

	switch sub.Name {
	case subChannel:
		return c.runChannel(context, sub)

	case subBrief:
		return runBrief(context, sub)

	case subStatus:
		return c.runStatus(context)

	case subRole:
		return runRole(context, sub)

	case subForget:
		return c.runForget(context, sub)

	case subAbout:
		return c.runAbout(context, sub)

	case subWhy:
		return c.runWhy(context, sub)

	case subBackends:
		return c.runBackends(context, sub)

	default:
		return respond(s, e, fmt.Sprintf("Unknown subcommand: %s", sub.Name))
	}
}

func runBrief(context *cmdadapter.SlashInteractionContext, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	s, e, store := context.Session, context.Event, context.Storage

	if len(sub.Options) == 0 {
		brief := store.GetChatBrief(e.GuildID)
		if brief == "" {
			return respond(s, e,
				"She has not been told what this place is. "+
					"`/chat brief text:<a sentence>` is how she finds out.")
		}
		return respond(s, e, "She has been told:\n>>> "+brief)
	}

	brief := strings.TrimSpace(sub.Options[0].StringValue())
	if err := store.SetChatBrief(e.GuildID, brief); err != nil {
		return fmt.Errorf("chat: set brief: %w", err)
	}
	if brief == "" {
		return respond(s, e, "Cleared. She knows nothing about this place again.")
	}
	return respond(s, e, "Noted. She will keep that in mind.")
}

func (c *ChatCommand) runStatus(context *cmdadapter.SlashInteractionContext) error {
	s, e, store := context.Session, context.Event, context.Storage

	var b strings.Builder

	if name := c.Service.CharacterName(); name != "" {
		b.WriteString("**" + name + "**\n\n")
	}

	channels := store.GetChatChannels(e.GuildID)
	if len(channels) == 0 {
		b.WriteString("Listening in: nowhere yet — `/chat channel mode:answers` in a channel.\n")
	} else {
		b.WriteString("Listening in: ")
		for i, id := range channels {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "<#%s>", id)
		}
		b.WriteString("\n")
	}

	if today := c.Service.Today(e.GuildID); len(today) > 0 {
		b.WriteString("\n**Today**\n")
		b.WriteString(todayLine(today))
		b.WriteString("\n")
	}

	status := c.Service.Status()
	if status.Waiting > 0 || status.Queued > 0 {
		fmt.Fprintf(&b, "Owed replies: %d held, %d queued\n", status.Waiting, status.Queued)
	}

	b.WriteString("\n" + c.stateHere(e.GuildID, e.ChannelID))

	if conflicts := c.Service.Conflicts(e.GuildID); len(conflicts) > 0 {
		b.WriteString("\n**Said against her card** — the card stays canon; change it, or remove the line from `me.md`\n")
		for _, f := range conflicts {
			fmt.Fprintf(&b, "- she said: %s\n  card: %s\n", trimForQuote(f.Text, maxConflictChars), trimForQuote(f.Conflict, maxConflictChars))
		}
	}

	if len(status.Backends) > 0 {
		b.WriteString("\n**Backends**, in the order she tries them\n")
		b.WriteString(backendLines(status.Backends))
	}

	b.WriteString("\n-# Her memory is in `" + status.MemoryPath + "` on the host.")
	return respond(s, e, b.String())
}

// todayOrder is the order the day's counts are shown in: what she did, then
// how it went down, then what went wrong.
var todayOrder = []string{"answered", "reacted", "stayed quiet", "held for later", "dropped"}

// todayLine renders the day's counts on one line, leaving out the zeros.
func todayLine(counts map[string]int) string {
	var parts []string
	for _, name := range todayOrder {
		if n := counts[name]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", name, n))
		}
	}
	return strings.Join(parts, " · ")
}

// maxBackendErrorChars caps a quoted backend error. Discord refuses an embed
// over its own limit, and a relay can answer with an HTML error page from a
// proxy in front of it rather than one tidy JSON line.
const maxBackendErrorChars = 240

// trimForEmbed flattens an error onto one line and cuts it to something an
// embed will accept.
func trimForEmbed(msg string) string {
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > maxBackendErrorChars {
		return msg[:maxBackendErrorChars] + "…"
	}
	return msg
}

// unavailableMessage explains why there is nobody to let in.
//
// Two genuinely different situations, and telling them apart is the whole
// point: the persona was never switched on, or it was switched on and could
// not start. Reporting the first when it is the second is what sent an
// operator to check a setting they had already set.
func unavailableMessage(reason string) string {
	if reason == "" {
		return "The persona is switched off on this bot, so there is nobody to let in. " +
			"Set `CHAT_ENABLED=true` in the deployment settings and restart."
	}
	return "The persona is switched on but could not start, so there is nobody to " +
		"let in yet.\n\n" + reason
}

func respond(s *discordgo.Session, e *discordgo.InteractionCreate, msg string) error {
	return reply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: msg,
		Color:       reply.EmbedColor,
	})
}

// stateHere is how she is in one channel, as her memory has it: her mood,
// how she has been lately, her dossiers on the people in the conversation,
// what she means to do, and what she last made of something here. It is read
// from the same files the next prompt is built from.
func (c *ChatCommand) stateHere(guildID, channelID string) string {
	st := c.Service.StateIn(guildID, channelID)

	var b strings.Builder
	fmt.Fprintf(&b, "**How she is in <#%s>**\n", channelID)
	if st.Self.Mood != "" {
		fmt.Fprintf(&b, "Mood: %s", st.Self.Mood)
		if !st.Self.MoodAt.IsZero() {
			fmt.Fprintf(&b, " (<t:%d:R>)", st.Self.MoodAt.Unix())
		}
		b.WriteString("\n")
	}
	if st.Self.Lately != "" {
		b.WriteString("\n**Lately, in her words**\n> " + trimForQuote(st.Self.Lately, 700) + "\n")
	}

	if len(st.People) > 0 {
		b.WriteString("\n**The people here**\n")
		for _, p := range st.People {
			fmt.Fprintf(&b, "- **%s**", p.Name)
			if p.Feeling != "" {
				b.WriteString(" — " + p.Feeling)
			}
			if p.Between != "" {
				b.WriteString(": " + trimForQuote(p.Between, 200))
			}
			b.WriteString("\n")
		}
	}

	if len(st.Threads) > 0 {
		b.WriteString("\n**She means to**\n")
		for _, t := range st.Threads {
			b.WriteString("- " + t.Text)
			if t.Person.Name != "" {
				b.WriteString(" (" + t.Person.Name + ")")
			}
			if !t.Due.IsZero() {
				fmt.Fprintf(&b, " · <t:%d:R>", t.Due.Unix())
			}
			b.WriteString("\n")
		}
	}

	if j := st.Last; j != nil {
		fmt.Fprintf(&b, "\n**Last thing she made of something here** (<t:%d:R>)\n", j.At.Unix())
		if j.Read != "" {
			b.WriteString("Read it as: " + j.Read + "\n")
		}
		if j.Feel != "" {
			b.WriteString("Felt: " + j.Feel + "\n")
		}
	}

	b.WriteString("\n-# ")
	if st.Proactive {
		b.WriteString("may speak first here")
	} else {
		b.WriteString("only answers here")
	}
	if st.Self.Reflected.IsZero() {
		b.WriteString(" · has not reflected on a day yet")
	} else {
		fmt.Fprintf(&b, " · last reflected <t:%d:R>", st.Self.Reflected.Unix())
	}
	b.WriteString("\n")
	return b.String()
}

// trimForQuote shortens text for an embed, on a word.
// maxConflictChars caps each side of a conflict shown in /chat status.
const maxConflictChars = 160

func trimForQuote(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		cut := string(r[:max])
		if i := strings.LastIndex(cut, " "); i > 0 {
			cut = cut[:i]
		}
		return cut + "…"
	}
	return s
}

// runForget moves what she remembers about this server aside.
//
// A typed word rather than a button because this is the whole of who she is
// here: an administrator who has typed "yes" has at least read the sentence
// above it. Moved aside rather than deleted, so a mistake can be undone by
// whoever has the host; see memory.Store.Forget.
func (c *ChatCommand) runForget(
	context *cmdadapter.SlashInteractionContext,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) error {
	s, e := context.Session, context.Event

	var confirm string
	for _, opt := range sub.Options {
		if opt.Name == optConfirm {
			confirm = strings.TrimSpace(strings.ToLower(opt.StringValue()))
		}
	}
	if confirm != confirmPhrase {
		return respond(s, e, fmt.Sprintf(
			"Nothing was touched. To wipe what she remembers about this server, "+
				"run `/chat forget confirm:%s`.", confirmPhrase))
	}

	if _, err := c.Service.ForgetGuild(e.GuildID); err != nil {
		return fmt.Errorf("chat: forget: %w", err)
	}
	return respond(s, e,
		"Forgotten: how she sees herself here, everything she knew and thought about the people "+
			"in it, what happened, and what she meant to do. The files were moved aside on the host, "+
			"not deleted.\n\n"+
			"She still knows who is a regular and who is new — that is counted from messages, not "+
			"remembered — and who has let her come after them.")
}

// runAbout shows her dossier on one person, exactly as it will be put in
// front of her, and what the bot keeps about them besides.
//
// Shown to administrators because it is kept about members without asking
// them, and the people running a server should be able to see exactly what
// that amounts to rather than take it on trust.
func (c *ChatCommand) runAbout(
	context *cmdadapter.SlashInteractionContext,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) error {
	s, e := context.Session, context.Event

	var userID string
	for _, opt := range sub.Options {
		if opt.Name == optUser {
			userID, _ = opt.Value.(string)
		}
	}

	p, ok, counted := c.Service.About(e.GuildID, userID)
	if !ok && counted == nil {
		return respond(s, e, fmt.Sprintf("She has never seen <@%s> say anything.", userID))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**<@%s>**", userID)
	if counted != nil {
		fmt.Fprintf(&b, " — %d messages seen, first <t:%d:R>", counted.Messages, counted.FirstSeen.Unix())
	}
	b.WriteString("\n")

	if !ok {
		b.WriteString("\nShe has no file on them yet. It starts the first time she talks with them.\n")
	} else {
		if p.Feeling != "" {
			b.WriteString("How she feels about them: " + p.Feeling + "\n")
		}
		if p.Who != "" {
			b.WriteString("\n**Who they are**\n> " + trimForQuote(p.Who, 700) + "\n")
		}
		if p.Between != "" {
			b.WriteString("\n**Between them**\n> " + trimForQuote(p.Between, 500) + "\n")
		}
		if len(p.Notes) > 0 {
			b.WriteString("\n**Her notes**\n")
			notes := p.Notes
			if len(notes) > 8 {
				notes = notes[len(notes)-8:]
			}
			for _, n := range notes {
				b.WriteString("- ")
				if !n.Day.IsZero() {
					b.WriteString(n.Day.Format("2 Jan") + " · ")
				}
				b.WriteString(trimForQuote(n.Text, 200) + "\n")
			}
		}
		if !p.LastTalked.IsZero() {
			fmt.Fprintf(&b, "\nLast talked with her <t:%d:R>\n", p.LastTalked.Unix())
		}
	}

	if counted != nil && chatsvc.Consented(counted.Attention) {
		b.WriteString("\n**Lets her come after them**")
		if !counted.ReachedAt.IsZero() {
			fmt.Fprintf(&b, " · last reached out <t:%d:R>", counted.ReachedAt.Unix())
		}
		if counted.Unanswered > 0 {
			fmt.Fprintf(&b, " · %d unanswered", counted.Unanswered)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n-# Her file on them is `people/" + userID + ".md` in her memory, and can be edited by hand.")
	return respond(s, e, b.String())
}

// runChannel sets or shows how she behaves in this channel.
//
// One setting with three steps rather than three subcommands, because they
// were never independent: she cannot speak first where she does not read,
// and silencing a channel always took speaking first with it. Per channel,
// because what is welcome differs: a general channel can take her starting
// something, a support channel cannot.
func (c *ChatCommand) runChannel(
	context *cmdadapter.SlashInteractionContext,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) error {
	s, e, store := context.Session, context.Event, context.Storage

	var mode string
	for _, opt := range sub.Options {
		if opt.Name == optMode {
			mode = opt.StringValue()
		}
	}

	switch mode {
	case "":
		switch {
		case store.IsChatProactive(e.GuildID, e.ChannelID):
			return respond(s, e, fmt.Sprintf("In <#%s> she reads, answers, and may start things herself.", e.ChannelID))
		case store.IsChatChannel(e.GuildID, e.ChannelID):
			return respond(s, e, fmt.Sprintf("In <#%s> she reads and answers, and never speaks first.", e.ChannelID))
		default:
			return respond(s, e, fmt.Sprintf("She does not read <#%s>.", e.ChannelID))
		}

	case modeOff:
		if err := store.RemoveChatChannel(e.GuildID, e.ChannelID); err != nil {
			return respond(s, e, "She was not reading this channel to begin with.")
		}
		// Drop what she is still holding, not just her permission to read
		// on: the live conversation would otherwise still go to a backend
		// the next time something she started came here.
		c.Service.Forget(e.ChannelID)
		return respond(s, e, fmt.Sprintf("She has stopped reading <#%s>.", e.ChannelID))

	case modeAnswers, modeFirst:
		// Adding a channel she already reads is not an error here: the
		// point of the mode is where it ends up, not the step taken.
		_ = store.AddChatChannel(e.GuildID, e.ChannelID)
		if err := store.SetChatProactive(e.GuildID, e.ChannelID, mode == modeFirst); err != nil {
			if errors.Is(err, storage.ErrChatChannelRequired) {
				return respond(s, e, "She could not be let into this channel.")
			}
			return fmt.Errorf("chat: set channel mode: %w", err)
		}
		msg := fmt.Sprintf("She reads <#%s> now, and answers when she wants to.", e.ChannelID)
		if mode == modeFirst {
			msg = fmt.Sprintf("She reads <#%s> now, and may start things herself: following up "+
				"on something someone told her, saying something into a room gone quiet, now "+
				"and then joining in when she overhears something. Only with a reason she "+
				"would stand behind, never at night, and a few times a day at most.", e.ChannelID)
		}
		return respond(s, e, msg+"\n\nEverything posted here is sent to the configured "+
			"model to produce her replies. `/chat channel mode:off` takes it back.")

	default:
		return respond(s, e, fmt.Sprintf("Unknown mode: %s", mode))
	}
}

// runRole sets or shows what a Discord role means here, in words she is told
// verbatim about anyone wearing it.
//
// Per role rather than per person because a server that has roles has already
// decided who is what. Words rather than a number: "a submissive here, speak
// to them as one" is a thing no number encodes, and a number rendered as an
// instruction is what made v1 a caricature.
func runRole(
	context *cmdadapter.SlashInteractionContext,
	sub *discordgo.ApplicationCommandInteractionDataOption,
) error {
	s, e, store := context.Session, context.Event, context.Storage

	var roleID, note string
	var setting bool
	for _, opt := range sub.Options {
		switch opt.Name {
		case optRole:
			roleID = opt.Value.(string)
		case optNote:
			note = strings.TrimSpace(opt.StringValue())
			setting = true
		}
	}
	if roleID == "" {
		return respond(s, e, "Name a role.")
	}

	if !setting {
		bias, ok := store.ChatRoleBiases(e.GuildID)[roleID]
		if !ok || strings.TrimSpace(bias.Note) == "" {
			return respond(s, e, fmt.Sprintf(
				"<@&%s> means nothing to her in particular. "+
					"`/chat role role:<role> note:<what they are here>` changes that.", roleID))
		}
		return respond(s, e, fmt.Sprintf("About anyone with <@&%s>, she is told:\n> %s", roleID, bias.Note))
	}

	if err := store.SetChatRoleBias(e.GuildID, roleID, storage.ChatRoleBias{Note: note}); err != nil {
		return fmt.Errorf("chat: set role note: %w", err)
	}
	if note == "" {
		return respond(s, e, fmt.Sprintf("<@&%s> means nothing to her again.", roleID))
	}
	return respond(s, e, fmt.Sprintf("About anyone with <@&%s>, she is told:\n> %s", roleID, note))
}
