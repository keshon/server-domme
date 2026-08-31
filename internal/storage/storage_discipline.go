package storage

import "fmt"

// SetPunishRole records the role id used for one discipline role type
// ("punisher", "victim", "assigned").
func (s *Storage) SetPunishRole(guildID string, roleType string, roleID string) error {
	g := s.guildSettings(guildID)
	if g.DisciplineRoles == nil {
		g.DisciplineRoles = map[string]string{}
	}
	g.DisciplineRoles[roleType] = roleID
	return s.settings.Put(g)
}

// GetPunishRole returns the role id for a discipline role type, or an error
// when the guild has not set one.
func (s *Storage) GetPunishRole(guildID string, roleType string) (string, error) {
	roleID, exists := s.guildSettings(guildID).DisciplineRoles[roleType]
	if !exists {
		return "", fmt.Errorf("storage: role type '%s' not set for this guild", roleType)
	}
	return roleID, nil
}
