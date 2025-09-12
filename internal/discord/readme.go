package discord

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"text/template"

	"server-domme/internal/command"
	"server-domme/internal/core"
)

type CommandDoc struct {
	Group    string
	Category string
	Name     string
	Desc     string
}

type TemplateData struct {
	CommandSections string
}

func updateReadme() error {
	commands := core.AllCommands()

	sort.Slice(commands, func(i, j int) bool {
		wi := command.CategoryWeights[commands[i].Category()]
		wj := command.CategoryWeights[commands[j].Category()]
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
		buf.WriteString(fmt.Sprintf("- **/%s** — %s\n", cmd.Name(), cmd.Description()))
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
