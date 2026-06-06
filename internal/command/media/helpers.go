package media

import (
	"fmt"
	"strings"

	"github.com/keshon/server-domme/internal/storage"
)

func sanitizeCategory(cat string) string {
	cat = strings.TrimSpace(cat)
	if cat == "" {
		return ""
	}
	cat = strings.ToLower(cat)
	cat = strings.ReplaceAll(cat, " ", "_")
	return cat
}

func categoryOrDefault(cat string) string {
	if cat == "" {
		return "all"
	}
	return cat
}

func resolveCategory(st *storage.Storage, guildID, requested string) (string, error) {
	category := sanitizeCategory(requested)
	if category != "" {
		return category, nil
	}
	if st == nil {
		return "", fmt.Errorf("no category specified and storage unavailable")
	}
	defCat, err := st.GetMediaDefault(guildID)
	if err != nil {
		return "", err
	}
	if defCat == "" {
		return "", fmt.Errorf("no category specified and no default set")
	}
	return defCat, nil
}

func validateRegisteredCategory(st *storage.Storage, guildID, category string) error {
	if st == nil {
		return fmt.Errorf("storage unavailable")
	}
	cats, err := st.GetMediaCategories(guildID)
	if err != nil {
		return err
	}
	for _, c := range cats {
		if c == category {
			return nil
		}
	}
	return fmt.Errorf("category `%s` is not registered; add it with /manage-media add-category", category)
}
