package commands

import (
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           61,
		Name:           "roll",
		Category:       "🎲 Game Mechanics",
		Description:    "Roll dice with crazy formulas like `2d6+1d4*2`",
		DCSlashHandler: rollSlashHandler,
		SlashOptions: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "formula",
				Description: "Supports `2d6+1d4*2-3` and similar math",
				Required:    true,
			},
		},
	})
}

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

func rollSlashHandler(ctx *SlashContext) {
	if !RequireGuild(ctx) {
		return
	}
	s, interaction := ctx.Session, ctx.InteractionCreate
	options := interaction.ApplicationCommandData().Options

	formula := ""
	for _, opt := range options {
		if opt.Name == "formula" {
			formula = strings.ReplaceAll(opt.StringValue(), " ", "")
		}
	}

	tokens := tokenRegex.FindAllString(formula, -1)
	if len(tokens) == 0 {
		respondEphemeral(s, interaction, "Can't parse your formula. Try something like `2d6+1d4*2-3`")
		return
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
			respondEphemeral(s, interaction, fmt.Sprintf("Failed to evaluate `%s`: %v", token, err))
			return
		}

		terms = append(terms, term{
			value:  val,
			desc:   desc,
			op:     currentOp,
			isDice: strings.Contains(desc, "["),
		})
	}

	// * and / first
	var merged []term
	for i := 0; i < len(terms); i++ {
		t := terms[i]
		if t.op == "*" || t.op == "/" {
			if len(merged) == 0 {
				respondEphemeral(s, interaction, "Syntax error: operator without left operand")
				return
			}
			prev := merged[len(merged)-1]
			merged = merged[:len(merged)-1]

			var newVal int
			switch t.op {
			case "*":
				newVal = prev.value * t.value
			case "/":
				if t.value == 0 {
					respondEphemeral(s, interaction, "Division by zero is forbidden. Even in games.")
					return
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

	// + and -
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
			respondEphemeral(s, interaction, "Unexpected operator during evaluation. Blame the dev.")
			return
		}
	}

	pretty := strings.Join(details, "")

	embed := &discordgo.MessageEmbed{
		Title:       "🎲 Dice Roll",
		Description: fmt.Sprintf("**User Input**:\t`%s`\n**Calculation**:\t%s\n**Result**:\t**%d**", formula, pretty, total),
		Color:       0x00cc99,
	}

	_ = s.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})

	err := logCommand(s, ctx.Storage, interaction.GuildID, interaction.ChannelID, interaction.Member.User.ID, interaction.Member.User.Username, "roll "+formula)
	if err != nil {
		log.Println("Failed to log /roll:", err)
	}
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

	// plain number
	num, err := strconv.Atoi(token)
	if err != nil {
		return 0, "", fmt.Errorf("not a number or dice")
	}
	return num, fmt.Sprintf("`%d`", num), nil
}
