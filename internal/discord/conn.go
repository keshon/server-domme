package discord

import (
	"github.com/disgoorg/disgo/bot"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

// conn is the live connection.
//
// It is replaced wholesale when a session opens and cleared when one closes,
// so a caller either gets a live connection or nothing — never a half-torn
// one. Everything above it outlives any single session and reaches Discord
// only through API.
type conn struct {
	client *bot.Client
}

// API is the neutral surface over this connection.
func (c *conn) API() adapter.BotAPI {
	return reply.NewSessionAPI(c.client)
}

// setConn publishes a live connection; clearConn withdraws it.
func (b *Bot) setConn(c *conn) { b.conn.Store(c) }
func (b *Bot) clearConn()      { b.conn.Store(nil) }

// currentConn is the live connection, or nil between sessions.
func (b *Bot) currentConn() *conn { return b.conn.Load() }
