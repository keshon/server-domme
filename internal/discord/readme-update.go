package discord

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"server-domme/internal/command"
	"server-domme/internal/config"
	"server-domme/pkg/cmd"
	"sort"
	"text/template"
)

// CommandDoc is a command documentation
type CommandDoc struct {
	Group    string
	Category string
	Name     string
	Desc     string
}

// TemplateData is a template data
type TemplateData struct {
	CommandSections string
}

// updateReadme updates the README.md file
func updateReadme() error {
	commands := cmd.DefaultRegistry.GetAll()
	sort.Slice(commands, func(i, j int) bool {
		metaI, _ := cmd.Root(commands[i]).(command.DiscordMeta)
		metaJ, _ := cmd.Root(commands[j]).(command.DiscordMeta)
		catI, catJ := "", ""
		if metaI != nil {
			catI = metaI.Category()
		}
		if metaJ != nil {
			catJ = metaJ.Category()
		}
		wi := config.CategoryWeights[catI]
		wj := config.CategoryWeights[catJ]
		if wi == wj {
			return commands[i].Name() < commands[j].Name()
		}
		return wi < wj
	})

	var buf bytes.Buffer
	currentCategory := ""
	for _, c := range commands {
		meta, _ := cmd.Root(c).(command.DiscordMeta)
		cat := ""
		if meta != nil {
			cat = meta.Category()
		}
		if cat != currentCategory {
			if currentCategory != "" {
				buf.WriteString("\n")
			}
			currentCategory = cat
			buf.WriteString(fmt.Sprintf("### %s\n\n", currentCategory))
		}

		name := c.Name()
		display := name
		if !(hasSpace(name) || startsWithUpper(name)) {
			display = "/" + display
		}
		buf.WriteString(fmt.Sprintf("- **%s** — %s\n", display, c.Description()))
	}

	tmplPath := filepath.Join(".", "README.md.tmpl")
	outPath := filepath.Join(".", "README.md")

	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return err
	}

	data := TemplateData{
		CommandSections: buf.String(),
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return err
	}

	log.Println("[INFO] README.md updated with current commands")
	return nil
}

// hasSpace returns true if the string contains a space
func hasSpace(s string) bool {
	for _, r := range s {
		if r == ' ' {
			return true
		}
	}
	return false
}

// startsWithUpper returns true if the string starts with an uppercase letter
func startsWithUpper(s string) bool {
	if s == "" {
		return false
	}
	r := rune(s[0])
	return r >= 'A' && r <= 'Z'
}
