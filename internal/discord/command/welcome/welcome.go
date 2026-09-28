// Package welcome is /welcome: introducing and welcoming a new member the way
// this server does it, on an administrator's say-so.
//
// Not automatic, by request. Roles on this kind of server are picked after
// joining, sometimes changed, and occasionally given by mistake; a person
// deciding "this one is ready" is the guard no event can replace. The command
// then does the fiddly part — right channel, right text for their role, the
// tag, a gif — and refuses rather than guesses whenever something is off.
package welcome

import (
	"context"
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/perm"
	"github.com/keshon/server-domme/internal/discord/reply"
	"github.com/keshon/server-domme/internal/storage"
)

// Subcommands and options.
const (
	subMember    = "member"
	subSetup     = "setup"
	subTemplate  = "template"
	subPreview   = "preview"
	subRoles     = "roles"
	subRemove    = "remove"
	subMove      = "move"
	subGifAdd    = "gif-add"
	subGifRemove = "gif-remove"
	subGifs      = "gifs"

	optUser          = "user"
	optRole          = "role"
	optAgain         = "again"
	optPart          = "part"
	optIntro         = "intro_channel"
	optWelcome       = "welcome_channel"
	optKind          = "kind"
	optURL           = "url"
	optFrom          = "from"
	optTo            = "to"
	optKeep          = "keep"
	optIntroNotify   = "intro_notify"
	optWelcomeNotify = "welcome_notify"

	kindIntro   = "intro"
	kindWelcome = "welcome"

	// modalPrefix starts the customID of the template editor, followed by
	// the kind and the role: "welcome:tpl:intro:123".
	modalPrefix = "welcome:tpl:"
	modalField  = "template"
)

// WelcomeCommand introduces and welcomes members.
type WelcomeCommand struct{}

func (c *WelcomeCommand) Name() string { return "welcome" }
func (c *WelcomeCommand) Description() string {
	return "Introduce and welcome a member, the way this server does it"
}
func (c *WelcomeCommand) Group() string    { return "welcome" }
func (c *WelcomeCommand) Category() string { return "👋 Welcome" }

// UserPermissions keeps the whole command with administrators: it posts in
// the server's name and tags people, and its templates are the server's
// first words to every newcomer.
func (c *WelcomeCommand) UserPermissions() []int64 {
	return []int64{perm.Administrator}
}

func roleOption(required bool, what string) adapter.SlashOption {
	return adapter.SlashOption{
		Type: adapter.OptionRole, Name: optRole,
		Description: what, Required: required,
	}
}

func channelOption(name, what string) adapter.SlashOption {
	return adapter.SlashOption{
		Type: adapter.OptionChannel, Name: name,
		Description: what, Required: false,
	}
}

func (c *WelcomeCommand) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionSubCommand,
				Name:        subMember,
				Description: "Post their role's intro and welcome — checks everything first",
				Options: []adapter.SlashOption{
					{Type: adapter.OptionUser, Name: optUser, Description: "Who to welcome", Required: true},
					roleOption(false, "Which of their roles to welcome them as, if they have more than one"),
					{
						Type:        adapter.OptionString,
						Name:        optPart,
						Description: "Only one of the two this time — both, left empty",
						Required:    false,
						Choices: []adapter.SlashChoice{
							{Name: "intro only", Value: kindIntro},
							{Name: "welcome only", Value: kindWelcome},
						},
					},
					{Type: adapter.OptionBoolean, Name: optAgain, Description: "Post again even if they were already welcomed for this role", Required: false},
				},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        subSetup,
				Description: "Where a role's intro and welcome go. Leave both empty to see",
				Options: []adapter.SlashOption{
					roleOption(true, "The role"),
					channelOption(optIntro, "Where the intro is posted"),
					channelOption(optWelcome, "Where the welcome is posted"),
					{Type: adapter.OptionBoolean, Name: optIntroNotify, Description: "Let @everyone, @here and role mentions in the intro ping", Required: false},
					{Type: adapter.OptionBoolean, Name: optWelcomeNotify, Description: "Let @everyone, @here and role mentions in the welcome ping", Required: false},
				},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        subTemplate,
				Description: "Write a role's intro or welcome text — paste it straight from Discord",
				Options: []adapter.SlashOption{
					roleOption(true, "The role"),
					{
						Type:        adapter.OptionString,
						Name:        optKind,
						Description: "Which text",
						Required:    true,
						Choices: []adapter.SlashChoice{
							{Name: "intro", Value: kindIntro},
							{Name: "welcome", Value: kindWelcome},
						},
					},
				},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        subPreview,
				Description: "See a role's intro and welcome as they would be posted, without posting",
				Options: []adapter.SlashOption{
					roleOption(true, "The role"),
					{Type: adapter.OptionUser, Name: optUser, Description: "Who to preview it for — you, if empty"},
				},
			},
			{Type: adapter.OptionSubCommand, Name: subRoles, Description: "Every role with a welcome set up"},
			{
				Type:        adapter.OptionSubCommand,
				Name:        subMove,
				Description: "Give a role's welcome to another role — a test setup to the real one",
				Options: []adapter.SlashOption{
					{Type: adapter.OptionRole, Name: optFrom, Description: "The role that has the welcome now", Required: true},
					{Type: adapter.OptionRole, Name: optTo, Description: "The role to give it to — one with no welcome yet", Required: true},
					channelOption(optIntro, "Post its intro here instead"),
					channelOption(optWelcome, "Post its welcome here instead"),
					{Type: adapter.OptionBoolean, Name: optKeep, Description: "Keep the first role's welcome too, making a copy"},
				},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        subRemove,
				Description: "Remove a role's welcome settings",
				Options:     []adapter.SlashOption{roleOption(true, "The role")},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        subGifAdd,
				Description: "Add a gif for a role's welcomes, or to the shared pool without a role",
				Options: []adapter.SlashOption{
					{Type: adapter.OptionString, Name: optURL, Description: "A link to a gif (tenor, giphy, a .gif)", Required: true},
					roleOption(false, "Whose welcomes get it — the shared pool, if empty"),
				},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        subGifRemove,
				Description: "Remove a gif from a role's welcomes, or from the shared pool without a role",
				Options: []adapter.SlashOption{
					{Type: adapter.OptionString, Name: optURL, Description: "The link to remove", Required: true},
					roleOption(false, "Whose gif it is — the shared pool, if empty"),
				},
			},
			{
				Type:        adapter.OptionSubCommand,
				Name:        subGifs,
				Description: "The gifs welcomes pick from — every pool, or one role's",
				Options:     []adapter.SlashOption{roleOption(false, "Just this role's")},
			},
		},
	}
}

func (c *WelcomeCommand) Run(ctx *adapter.SlashInteractionContext) error {
	sub, ok := ctx.FirstOption()
	if !ok {
		return respond(ctx, "Pick something: `member`, `setup`, `template`, `preview`, `roles`, `move`, `remove`, `gif-add`, `gif-remove` or `gifs`.")
	}
	opts := optionsOf(sub)

	switch sub.Name {
	case subMember:
		return runMember(ctx, opts)
	case subSetup:
		return runSetup(ctx, opts)
	case subTemplate:
		return runTemplate(ctx, opts)
	case subPreview:
		return runPreview(ctx, opts)
	case subRoles:
		return runRoles(ctx)
	case subMove:
		return runMove(ctx, opts)
	case subRemove:
		return runRemove(ctx, opts)
	case subGifAdd, subGifRemove, subGifs:
		return runGifs(ctx, sub.Name, opts)
	default:
		return respond(ctx, "Unknown subcommand: "+sub.Name)
	}
}

type options = map[string]adapter.SlashArgument

func optionsOf(sub adapter.SlashArgument) options {
	out := make(options, len(sub.Options))
	for _, o := range sub.Options {
		out[o.Name] = o
	}
	return out
}

func optString(opts options, name string) string {
	if o, ok := opts[name]; ok {
		return o.StringValue()
	}
	return ""
}

func optBool(opts options, name string) bool {
	if o, ok := opts[name]; ok {
		return o.BoolValue()
	}
	return false
}

func roleIDOf(opts options) string {
	return optString(opts, optRole)
}

func respond(ctx *adapter.SlashInteractionContext, msg string) error {
	return ctx.RespondEphemeral(&adapter.Embed{Description: msg, Color: reply.EmbedColor})
}

// runMember welcomes one person. Every check runs before anything is posted,
// and each part — intro, welcome — is checked, posted and recorded on its own,
// so a run that fails half way can be repeated to finish the other half
// without posting the first half twice.
func runMember(ctx *adapter.SlashInteractionContext, opts options) error {
	// Fetching the member and posting two messages can pass Discord's three
	// seconds to answer an interaction; acknowledge first.
	if err := ctx.DeferEphemeral(); err != nil {
		return err
	}
	finish := func(msg string) error {
		return ctx.FollowupEphemeral(&adapter.Embed{Description: msg, Color: reply.EmbedColor})
	}

	userID := optString(opts, optUser)
	member, err := ctx.API.Member(ctx.GuildID(), userID)
	if err != nil {
		return finish(fmt.Sprintf("<@%s> is not in this server, so there is nobody to welcome.", userID))
	}
	if member.Bot {
		return finish("That is a bot. Bots do not get welcomed.")
	}

	roleID, problem := pickRole(ctx.Storage, ctx.GuildID(), member, opts[optRole])
	if problem != "" {
		return finish(problem)
	}
	cfg := ctx.Storage.WelcomeRoleFor(ctx.GuildID(), roleID)

	guildName := "the server"
	if info, err := ctx.API.GuildInfo(ctx.GuildID()); err == nil {
		guildName = info.Name
	}
	v := Vars{UserID: userID, Name: member.DisplayName(), Server: guildName, Role: roleName(ctx, roleID)}
	channels, _ := ctx.API.GuildChannels(ctx.GuildID())
	again := optBool(opts, optAgain)
	only := optString(opts, optPart)
	done := ctx.Storage.WelcomedFor(ctx.GuildID(), userID, roleID)

	intro := planPart(ctx, ctx.GuildID(), "Intro", cfg.IntroChannel, cfg.IntroTemplate, v, channels, "", cfg.IntroNotifyAll)
	welcome := planPart(ctx, ctx.GuildID(), "Welcome", cfg.WelcomeChannel, cfg.WelcomeTemplate, v, channels, randomGif(welcomeGifs(ctx.Storage, ctx.GuildID(), roleID)), cfg.WelcomeNotifyAll)
	// One part on its own: the half of a welcome that failed is posted
	// without posting the half that went out.
	switch only {
	case kindIntro:
		welcome.notThisTime()
	case kindWelcome:
		intro.notThisTime()
	}
	if welcome.ok() && welcome.gif != "" {
		welcome.file = gifFile(ctx.Storage, ctx.GuildID(), welcome.gif)
	}
	if done != nil && !again {
		if !done.IntroAt.IsZero() && intro.ok() {
			intro.skip = fmt.Sprintf("already posted <t:%d:R> by <@%s> — add `again:True` to post it again", done.IntroAt.Unix(), done.By)
		}
		if !done.WelcomeAt.IsZero() && welcome.ok() {
			welcome.skip = fmt.Sprintf("already posted <t:%d:R> by <@%s> — add `again:True` to post it again", done.WelcomeAt.Unix(), done.By)
		}
	}

	if !intro.ok() && !welcome.ok() {
		return finish(fmt.Sprintf("Nothing was posted for <@%s> as <@&%s>.\n\n%s\n%s", userID, roleID, intro.report(), welcome.report()))
	}

	now := time.Now()
	introSent := intro.send(ctx, userID)
	welcomeSent := welcome.send(ctx, userID)
	if introSent || welcomeSent {
		if err := ctx.Storage.MarkWelcomed(ctx.GuildID(), userID, roleID, ctx.UserID(), introSent, welcomeSent, now); err != nil {
			ctx.AppLog.Warn().Err(err).Str("guild_id", ctx.GuildID()).Msg("welcome_record_failed")
		}
	}

	return finish(fmt.Sprintf("**<@%s> as <@&%s>**\n%s\n%s", userID, roleID, intro.report(), welcome.report()))
}

// pickRole decides which of someone's roles to welcome them as, or explains
// why it cannot.
func pickRole(store *storage.Storage, guildID string, member *adapter.Member, opt adapter.SlashArgument) (string, string) {
	configured := store.WelcomeRoles(guildID)
	if len(configured) == 0 {
		return "", "No role has a welcome set up yet. Start with `/welcome setup` and `/welcome template`."
	}

	if opt.Name != "" {
		roleID := opt.StringValue()
		if store.WelcomeRoleFor(guildID, roleID) == nil {
			return "", fmt.Sprintf("<@&%s> has no welcome set up. `/welcome roles` lists the ones that do.", roleID)
		}
		// The role is the reason for everything posted, so they have to
		// actually have it: welcoming someone as a Domme who is not one is
		// the mistake this command most needs to refuse.
		if !slices.Contains(member.Roles, roleID) {
			return "", fmt.Sprintf("<@%s> does not have <@&%s>. Give them the role first, then welcome them.", member.UserID, roleID)
		}
		return roleID, ""
	}

	var theirs []string
	for _, w := range configured {
		if slices.Contains(member.Roles, w.RoleID) {
			theirs = append(theirs, w.RoleID)
		}
	}
	switch len(theirs) {
	case 1:
		return theirs[0], ""
	case 0:
		return "", fmt.Sprintf("<@%s> has none of the roles with a welcome set up (%s). Give them one first.",
			member.UserID, mentionRoles(configured))
	default:
		return "", fmt.Sprintf("<@%s> has more than one role with a welcome: %s. Say which with `role:`.",
			member.UserID, mentionRoleIDs(theirs))
	}
}

// part is one message to post: checked and rendered before anything is sent.
type part struct {
	ctx       *adapter.SlashInteractionContext
	label     string
	channelID string
	content   string
	gif       string
	// file is the gif to attach in place of the link; nil posts the link.
	file *gifAttachment
	// problem is why it cannot be posted, skip why it will not be, and
	// posted the link to it once it has been.
	problem, skip, posted string
	// note is something worth saying about a part that did go out.
	note string
	// notifyAll lets @everyone, @here and role mentions in the text ping.
	notifyAll bool
}

func (p *part) ok() bool { return p.problem == "" && p.skip == "" }

// notThisTime leaves a part out of the run: not asked for, so not posted,
// and not reported as something wrong.
func (p *part) notThisTime() {
	p.problem, p.skip = "", "not this time"
}

func (p *part) report() string {
	switch {
	case p.posted != "" && p.note != "":
		return fmt.Sprintf("✅ **%s** posted in <#%s> — %s\n⚠️ %s", p.label, p.channelID, p.posted, p.note)
	case p.posted != "":
		return fmt.Sprintf("✅ **%s** posted in <#%s> — %s", p.label, p.channelID, p.posted)
	case p.problem != "":
		return fmt.Sprintf("⚠️ **%s** not posted: %s", p.label, p.problem)
	case p.skip != "":
		return fmt.Sprintf("➖ **%s** %s", p.label, p.skip)
	default:
		return fmt.Sprintf("⚠️ **%s** not posted", p.label)
	}
}

// planPart checks and renders one part without sending it.
func planPart(ctx *adapter.SlashInteractionContext, guildID, label, channelID, template string, v Vars, channels []adapter.Channel, gif string, notifyAll bool) *part {
	p := &part{ctx: ctx, label: label, channelID: channelID, gif: gif, notifyAll: notifyAll}
	switch {
	case strings.TrimSpace(template) == "" && channelID == "":
		p.skip = "is not set up for this role"
		return p
	case strings.TrimSpace(template) == "":
		p.problem = "no text written yet — `/welcome template`"
		return p
	case channelID == "":
		p.problem = "no channel set — `/welcome setup`"
		return p
	}
	if why := ctx.API.CanPostIn(channelID, guildID); why != "" {
		p.problem = why
		return p
	}
	p.content = Render(template, v, channels)
	if TooLong(p.content) {
		p.problem = fmt.Sprintf("the text comes to %d characters with their name in it, over Discord's 2000", len([]rune(p.content)))
	}
	if notifyAll && pingsEveryoneOrHere(p.content) {
		if why := ctx.API.CanMentionEveryone(channelID); why != "" {
			p.problem = why
		}
	}
	return p
}

// send posts a planned part, and its gif after it when the two will not fit
// in one message. It reports whether the part went out.
func (p *part) send(ctx *adapter.SlashInteractionContext, userID string) bool {
	if !p.ok() {
		return false
	}
	content := p.content
	separateGif := false
	post := adapter.OutgoingMessage{
		Content:     content,
		MentionUser: userID,
	}
	if p.notifyAll && pingsEveryoneOrHere(content) {
		post.AllowEveryone = true
		post.AllowRoles = roleIDsIn(content)
	}
	if p.file != nil {
		post.File = p.file.reader()
		post.FileName = p.file.name
	} else if p.gif != "" {
		if joined := content + "\n" + p.gif; !TooLong(joined) {
			post.Content = joined
		} else {
			separateGif = true
		}
	}

	msgID, err := ctx.API.PostMessage(p.channelID, post)
	if err != nil && p.file != nil && isMissingPermissions(err) {
		// Attaching a file needs Attach Files, and the words matter more
		// than the gif: post them with the link instead, and say what was
		// missing.
		post.File = nil
		post.FileName = ""
		if joined := content + "\n" + p.gif; !TooLong(joined) {
			post.Content = joined
		} else {
			separateGif = true
		}
		p.note = fmt.Sprintf("the gif went as a link — I cannot attach files in <#%s>", p.channelID)
		msgID, err = ctx.API.PostMessage(p.channelID, post)
	}
	if err != nil {
		p.problem = refused(err, p.channelID)
		return false
	}
	if separateGif {
		_, _ = ctx.API.PostMessage(p.channelID, adapter.OutgoingMessage{Content: p.gif})
	}
	p.posted = fmt.Sprintf("https://discord.com/channels/%s/%s/%s", ctx.GuildID(), p.channelID, msgID)
	return true
}

// isMissingPermissions reports whether Discord refused something for want of
// a permission, as opposed to any other refusal.
func isMissingPermissions(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "missing permissions") ||
		strings.Contains(err.Error(), "50013")
}

// errMissingPermissions is Discord's code for "Missing Permissions".
const errMissingPermissions = 50013

// refused puts a Discord refusal in words an administrator can act on.
func refused(err error, channelID string) string {
	if isMissingPermissions(err) {
		return fmt.Sprintf("Discord would not let me post in <#%s>. Beyond View Channel and Send Messages, "+
			"posting a gif needs Attach Files, and a thread needs Send Messages in Threads.", channelID)
	}
	return "Discord refused it: " + err.Error()
}

var roleMention = regexp.MustCompile(`<@&(\d+)>`)

// roleIDsIn collects the role mentions in a text, for the mention allow-list.
func roleIDsIn(content string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, match := range roleMention.FindAllStringSubmatch(content, -1) {
		if id := match[1]; !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func pingsEveryoneOrHere(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "@everyone") || strings.Contains(lower, "@here")
}

func roleName(ctx *adapter.SlashInteractionContext, roleID string) string {
	if name, err := ctx.API.RoleName(ctx.GuildID(), roleID); err == nil {
		return name
	}
	return "member"
}

// unlinkedHint is said after names that matched nothing.
const unlinkedHint = " — those stay plain text. Archived threads are looked up when a text is " +
	"saved; if one is still not found, paste the thread's link into the template, and Discord shows it as a link."

// linkArchived writes links to archived threads into a template being
// saved, for "#names" nothing open matches.
func linkArchived(ctx *adapter.SlashInteractionContext, text string, known []adapter.Channel) string {
	return linkArchivedOn(ctx.API, ctx.GuildID(), text, known)
}

// linkArchivedModal is the same for the template editor: the modal context
// carries the same connection the slash one does.
func linkArchivedModal(ctx *adapter.ModalSubmitContext, text string, known []adapter.Channel) string {
	return linkArchivedOn(ctx.API, ctx.GuildID(), text, known)
}

func linkArchivedOn(api adapter.SessionAPI, guildID, text string, known []adapter.Channel) string {
	if len(unlinked(Render(text, Vars{}, known))) == 0 {
		return text
	}
	taken := make(map[string]bool, len(known))
	for _, c := range known {
		taken[strings.ToLower(c.Name)] = true
	}
	var archived []adapter.Channel
	threads, _ := api.GuildArchivedThreads(guildID)
	for _, t := range threads {
		if !taken[strings.ToLower(t.Name)] {
			archived = append(archived, t)
		}
	}
	if len(archived) == 0 {
		return text
	}
	return linkChannels(invisible.Replace(text), archived)
}

// gifMedia is the gif file behind a link: remembered, or looked up now and
// remembered. "" when none could be found.
func gifMedia(store *storage.Storage, guildID, link string) string {
	if media := store.WelcomeGifMedia(guildID, link); media != "" {
		return media
	}
	ctx, cancel := context.WithTimeout(context.Background(), gifClient.Timeout)
	defer cancel()
	media, err := resolveGif(ctx, gifClient, link)
	if err != nil {
		return ""
	}
	_ = store.SetWelcomeGifMedia(guildID, link, media)
	return media
}

// gifFile is the gif to attach to a welcome, or nil to post the link.
func gifFile(store *storage.Storage, guildID, link string) *gifAttachment {
	media := gifMedia(store, guildID, link)
	if media == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), gifClient.Timeout)
	defer cancel()
	f, err := fetchGif(ctx, gifClient, media)
	if err != nil {
		return nil
	}
	return f
}

// welcomeGifs is the pool a role's welcome picks from.
func welcomeGifs(store *storage.Storage, guildID, roleID string) []string {
	gifs, _ := store.WelcomeGifPool(guildID, roleID)
	return gifs
}

func randomGif(gifs []string) string {
	if len(gifs) == 0 {
		return ""
	}
	return gifs[rand.IntN(len(gifs))]
}

func mentionRoles(roles []storage.WelcomeRole) string {
	ids := make([]string, 0, len(roles))
	for _, r := range roles {
		ids = append(ids, r.RoleID)
	}
	return mentionRoleIDs(ids)
}

func mentionRoleIDs(ids []string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, "<@&"+id+">")
	}
	return strings.Join(parts, ", ")
}

// unlinked finds "#names" a rendered text still carries, which match no
// channel — a renamed channel, or a typo — so they can be flagged before the
// text is ever posted.
var unlinkedChannel = regexp.MustCompile(`(?:^|\s)#([^\s#<>.,!?;:]+)`)

func unlinked(rendered string) []string {
	var out []string
	for _, m := range unlinkedChannel.FindAllStringSubmatchIndex(rendered, -1) {
		name := "#" + rendered[m[2]:m[3]]
		if rest := rendered[m[3]:]; strings.HasPrefix(rest, " ") {
			if next, _ := utf8.DecodeRuneInString(strings.TrimLeft(rest, " ")); isNameRune(next) {
				name += "…"
			}
		}
		out = append(out, name)
	}
	return out
}

// isAdmin re-checks a modal submitter: submissions arrive without the
// command's permission check, which only ran when the modal was opened.
func isAdmin(ctx *adapter.ModalSubmitContext) bool {
	if ctx.Config != nil && ctx.Config.DeveloperID != "" && ctx.Config.DeveloperID == ctx.UserID() {
		return true
	}
	perms, err := ctx.MemberPermissions()
	if err != nil {
		return false
	}
	return perms&perm.Administrator != 0
}
