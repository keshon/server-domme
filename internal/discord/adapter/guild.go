package adapter

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

// Message is one channel message: what /announce republishes and /translate
// reads before translating.
type Message struct {
	ID          string
	Content     string
	Embeds      []*Embed
	Attachments []Attachment
}

// Attachment is a file riding on a message. The bytes are fetched when the
// message is forwarded, not when it is read.
type Attachment struct {
	Name string
	URL  string
}

// GuildMember is one member for name resolution: username, server nickname
// and display name, each of which someone may have meant by @that-guy.
type GuildMember struct {
	UserID     string
	Username   string
	Nick       string
	GlobalName string
}
