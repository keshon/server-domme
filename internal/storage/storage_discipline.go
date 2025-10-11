package storage

import "fmt"

func (s *Storage) SetPunishRole(guildID string, roleType string, roleID string) error {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return err
	}

	if record.DisciplineRoles == nil {
		record.DisciplineRoles = map[string]string{}
	}

	record.DisciplineRoles[roleType] = roleID
	s.ds.Add(guildID, record)
	return nil
}

func (s *Storage) GetPunishRole(guildID string, roleType string) (string, error) {
	record, err := s.getOrCreateGuildRecord(guildID)
	if err != nil {
		return "", err
	}

	roleID, exists := record.DisciplineRoles[roleType]
	if !exists {
		return "", fmt.Errorf("role type '%s' not set for this guild", roleType)
	}

	return roleID, nil
}
