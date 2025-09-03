package main

import (
	"bytes"
	"fmt"
	"os"
	"server-domme/internal/core"
	"sort"
	"text/template"
)

type CmdInfo struct {
	Name        string
	Description string
	Category    string
	Group       string
}

func main() {
	cmds := core.AllCommands()

	sections := make(map[string][]CmdInfo)
	for _, cmd := range cmds {
		info := CmdInfo{
			Name:        "/" + cmd.Name(),
			Description: cmd.Description(),
			Category:    cmd.Category(),
			Group:       cmd.Group(),
		}
		sections[info.Category] = append(sections[info.Category], info)
	}

	for _, cmds := range sections {
		sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
	}

	tmplData, err := os.ReadFile("README.md.tmpl")
	if err != nil {
		panic(err)
	}

	tmpl, err := template.New("readme").Parse(string(tmplData))
	if err != nil {
		panic(err)
	}

	var buf bytes.Buffer
	for cat, cmds := range sections {
		fmt.Fprintf(&buf, "### %s\n\n", cat)
		for _, c := range cmds {
			fmt.Fprintf(&buf, "* **`%s`**\n  %s\n\n", c.Name, c.Description)
		}
	}

	data := map[string]any{
		"CommandSections": buf.String(),
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		panic(err)
	}

	if err := os.WriteFile("README.md", out.Bytes(), 0644); err != nil {
		panic(err)
	}
}
