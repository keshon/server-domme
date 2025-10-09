package core

var registry = map[string]Command{}

// RegisterCommand registers a command
func RegisterCommand(cmd Command) {
	registry[cmd.Name()] = cmd
	for _, a := range cmd.Aliases() {
		registry[a] = cmd
	}
}

// GetCommand returns the command with the given name
func GetCommand(name string) (Command, bool) {
	cmd, ok := registry[name]
	return cmd, ok
}

// AllCommands returns all registered commands
func AllCommands() []Command {
	seen := map[string]bool{}
	list := make([]Command, 0)
	for _, cmd := range registry {
		if seen[cmd.Name()] {
			continue
		}
		list = append(list, cmd)
		seen[cmd.Name()] = true
	}
	return list
}
