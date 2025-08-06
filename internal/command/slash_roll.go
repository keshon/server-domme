package command

import (
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var (
	tokenRegex = regexp.MustCompile(`(?i)(\d*d\d+|\d+|[+\-*/])`)
	diceRegex  = regexp.MustCompile(`(?i)^(\d*)d(\d+)$`)
	validOps   = map[string]bool{"+": true, "-": true, "*": true, "/": true}
)

type term struct {
	value  int
	desc   string
	op     string
	isDice bool
}

type RollCommand struct{}

func (c *RollCommand) Name() string        { return "roll" }
func (c *RollCommand) Description() string { return "Roll dices like `2d20+1d6-2`" }
func (c *RollCommand) Aliases() []string   { return []string{} }

func (c *RollCommand) Group() string    { return "roll" }
func (c *RollCommand) Category() string { return "🎲 Game Mechanics" }

func (c *RollCommand) RequireAdmin() bool { return false }
func (c *RollCommand) RequireDev() bool   { return false }

func (c *RollCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Type:        discordgo.ChatApplicationCommand,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "formula",
				Description: "Supports `2d6+1d4*2-3` and similar math",
				Required:    true,
			},
		},
	}
}

func (c *RollCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event
	options := event.ApplicationCommandData().Options

	formula := ""
	for _, opt := range options {
		if opt.Name == "formula" {
			formula = strings.ReplaceAll(opt.StringValue(), " ", "")
		}
	}

	tokens := tokenRegex.FindAllString(formula, -1)
	if len(tokens) == 0 {
		return respondEphemeral(session, event, "Can't parse your formula. Try something like `2d6+1d4*2-3`")
	}

	var terms []term
	currentOp := "+"

	for _, token := range tokens {
		if validOps[token] {
			currentOp = token
			continue
		}

		val, desc, err := evaluateToken(token)
		if err != nil {
			return respondEphemeral(session, event, fmt.Sprintf("Failed to evaluate `%s`: %v", token, err))
		}

		terms = append(terms, term{
			value:  val,
			desc:   desc,
			op:     currentOp,
			isDice: strings.Contains(desc, "["),
		})
	}

	var merged []term
	for i := 0; i < len(terms); i++ {
		t := terms[i]
		if t.op == "*" || t.op == "/" {
			if len(merged) == 0 {
				return respondEphemeral(session, event, "Syntax error: operator without left operand")
			}
			prev := merged[len(merged)-1]
			merged = merged[:len(merged)-1]

			var newVal int
			switch t.op {
			case "*":
				newVal = prev.value * t.value
			case "/":
				if t.value == 0 {
					return respondEphemeral(session, event, "Division by zero is forbidden. Even in games.")
				}
				newVal = prev.value / t.value
			}

			newDesc := fmt.Sprintf("%s %s %s", prev.desc, t.op, t.desc)
			merged = append(merged, term{
				value:  newVal,
				desc:   newDesc,
				op:     prev.op,
				isDice: prev.isDice || t.isDice,
			})
		} else {
			merged = append(merged, t)
		}
	}

	total := 0
	var details []string
	for _, t := range merged {
		if len(details) > 0 {
			details = append(details, fmt.Sprintf(" %s ", t.op))
		}
		details = append(details, t.desc)

		switch t.op {
		case "+":
			total += t.value
		case "-":
			total -= t.value
		default:
			return respondEphemeral(session, event, "Unexpected operator during evaluation. Blame the dev.")
		}
	}

	pretty := strings.Join(details, "")

	embed := &discordgo.MessageEmbed{
		Title:       "🎲 Dice Roll",
		Description: fmt.Sprintf("**User Input**:\t`%s`\n**Calculation**:\t%s\n**Result**:\t**%d**", formula, pretty, total),
		Color:       0x00cc99,
	}

	err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
	if err != nil {
		return err
	}

	err = logCommand(session, slash.Storage, event.GuildID, event.ChannelID, event.Member.User.ID, event.Member.User.Username, c.Name()+" "+formula)
	if err != nil {
		log.Println("Failed to log /roll:", err)
	}

	return nil
}

func evaluateToken(token string) (int, string, error) {
	if diceRegex.MatchString(token) {
		matches := diceRegex.FindStringSubmatch(token)
		countStr := matches[1]
		sidesStr := matches[2]

		count := 1
		if countStr != "" {
			n, err := strconv.Atoi(countStr)
			if err != nil {
				return 0, "", fmt.Errorf("invalid dice count")
			}
			count = n
		}

		sides, err := strconv.Atoi(sidesStr)
		if err != nil || sides < 2 {
			return 0, "", fmt.Errorf("invalid dice sides")
		}

		if count > 100 || sides > 1000 {
			return 0, "", fmt.Errorf("too big. max 100 dice, 1000 sides")
		}

		var sum int
		var rolls []string
		for i := 0; i < count; i++ {
			r := rand.Intn(sides) + 1
			sum += r
			rolls = append(rolls, strconv.Itoa(r))
		}
		return sum, fmt.Sprintf("`%s` [%s]", token, strings.Join(rolls, ", ")), nil
	}

	num, err := strconv.Atoi(token)
	if err != nil {
		return 0, "", fmt.Errorf("not a number or dice")
	}
	return num, fmt.Sprintf("`%d`", num), nil
}

func init() {
	Register(
		WithGroupAccessCheck()(
			WithGuildOnly(
				&RollCommand{},
			),
		),
	)
}
