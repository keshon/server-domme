package command

import (
	"fmt"
	"log"
)

type ChatCommand struct{}

func (c *ChatCommand) Name() string              { return "chat" }
func (c *ChatCommand) Description() string       { return "Responds when bot is mentioned" }
func (c *ChatCommand) Aliases() []string         { return []string{} }
func (c *ChatCommand) Group() string             { return "chat" }
func (c *ChatCommand) Category() string          { return "💬 Chat" }
func (c *ChatCommand) RequireAdmin() bool        { return false }
func (c *ChatCommand) RequireDev() bool          { return false }
func (c *ChatCommand) Run(ctx interface{}) error { return nil } // unused for message

func (c *ChatCommand) Message(ctx *MessageContext) error {
	user := ctx.Event.Author.Username
	msg := ctx.Event.Content

	// For now just print
	fmt.Printf("[CHAT] %s: %s\n", user, msg)

	// Example reply
	_, err := ctx.Session.ChannelMessageSend(ctx.Event.ChannelID,
		fmt.Sprintf("I heard you, %s 👀", user))
	if err != nil {
		log.Println("failed to send reply:", err)
	}
	return err
}

func init() {
	Register(&ChatCommand{})
}
