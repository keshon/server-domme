package command

import (
	"server-domme/internal/core"
	"sort"
)

func getUniqueGroups() []string {
	set := map[string]struct{}{}
	for _, cmd := range core.AllCommands() {
		group := cmd.Group()
		if group != "" {
			set[group] = struct{}{}
		}
	}
	var result []string
	for group := range set {
		result = append(result, group)
	}
	sort.Strings(result)
	return result
}
