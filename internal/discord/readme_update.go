package discord

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"text/template"

	"server-domme/internal/config"
	"server-domme/internal/registry"
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
	commands := registry.AllCommands()

	sort.Slice(commands, func(i, j int) bool {
		wi := config.CategoryWeights[commands[i].Category()]
		wj := config.CategoryWeights[commands[j].Category()]
		if wi == wj {
			return commands[i].Name() < commands[j].Name()
		}
		return wi < wj
	})

	var buf bytes.Buffer
	currentCategory := ""
	for _, cmd := range commands {
		if cmd.Category() != currentCategory {
			if currentCategory != "" {
				buf.WriteString("\n")
			}
			currentCategory = cmd.Category()
			buf.WriteString(fmt.Sprintf("### %s\n\n", currentCategory))
		}

		name := cmd.Name()
		display := name
		if !(hasSpace(name) || startsWithUpper(name)) {
			display = "/" + display
		}

		buf.WriteString(fmt.Sprintf("- **%s** — %s\n", display, cmd.Description()))
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
