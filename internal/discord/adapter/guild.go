package adapter

import (
	"io"
	"time"
)

// GuildInfo is what a command can learn about a guild without naming the
// library that fetched it.
//
// The counts are whatever the session could see: a cached guild reports the
// members it has cached rather than the guild's true size, which is what the
// status command has always shown. Reporting it as a plain int keeps that
// honest — a command cannot mistake this for a figure it could not have.
type GuildInfo struct {
	ID       string
	Name     string
	Members  int
	Roles    int
	Channels int
}

// Attachment is a file: uploaded with a command, or riding on a message. The
// bytes are fetched when the file is used, not when it is read.
type Attachment struct {
	ID   string
	Name string
	URL  string
}

// Message is one channel message: what /announce republishes and /translate
// reads before translating.
type Message struct {
	ID          string
	Content     string
	Embeds      []*Embed
	Attachments []Attachment
}

// ListedMessage is one message in a channel listing: id and time, which is
// all a purge selects on. Content and author are carried for readers like
// summarize; purges ignore them.
type ListedMessage struct {
	ID         string
	Timestamp  time.Time
	Content    string
	AuthorName string
	AuthorID   string
	Bot        bool
}

// GuildMember is one member for name resolution: username, server nickname
// and display name, each of which someone may have meant by @that-guy.
type GuildMember struct {
	UserID     string
	Username   string
	Nick       string
	GlobalName string
}

// Member is one guild member: who they are, what they wear, whether they
// are a bot. Welcomes refuse bots and check roles off this.
type Member struct {
	UserID     string
	Username   string
	GlobalName string
	Nick       string
	Roles      []string
	Bot        bool
}

// DisplayName is how they are shown in the server: nickname, display name,
// username, in that order.
func (m *Member) DisplayName() string {
	if m == nil {
		return "there"
	}
	switch {
	case m.Nick != "":
		return m.Nick
	case m.GlobalName != "":
		return m.GlobalName
	case m.Username != "":
		return m.Username
	default:
		return "there"
	}
}

// Channel is one channel or open thread a template may name.
type Channel struct {
	ID   string
	Name string
}

// OutgoingMessage is a channel post: words, at most one file, and who may
// be notified. Mentions default to nobody; the welcome passes its newcomer,
// plus everyone and roles only where the settings allow.
type OutgoingMessage struct {
	Content string
	// File is attached when non-nil; the caller keeps ownership of closing it.
	File     io.Reader
	FileName string
	// MentionUser is always notified.
	MentionUser string
	// AllowEveryone lets @everyone and @here in the text ping.
	AllowEveryone bool
	// AllowRoles lets these role mentions ping.
	AllowRoles []string
}
