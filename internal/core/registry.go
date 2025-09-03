package core

var registry = map[string]Command{}

func RegisterCommand(cmd Command) {
	registry[cmd.Name()] = cmd
	for _, a := range cmd.Aliases() {
		registry[a] = cmd
	}
}

func GetCommand(name string) (Command, bool) {
	cmd, ok := registry[name]
	return cmd, ok
}

func AllCommands() []Command {
	seen := map[string]bool{}
	var list []Command
	for _, cmd := range registry {
		if seen[cmd.Name()] {
			continue
		}
		list = append(list, cmd)
		seen[cmd.Name()] = true
	}
	return list
}
