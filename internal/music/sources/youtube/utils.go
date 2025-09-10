package youtube

import (
	"regexp"
	"strings"
)

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func isYouTubeURL(input string) bool {
	youtubeRegex := regexp.MustCompile(`(?:https?:\/\/)?(?:www\.)?(youtube\.com|youtu\.be)\/\S+`)
	return youtubeRegex.MatchString(input)
}

func isYouTubePlaylistURL(s string) bool {
	return strings.Contains(s, "list=")
}

func isYouTubeVideoURL(s string) bool {
	return strings.Contains(s, "youtube.com/watch?v=") || strings.Contains(s, "youtu.be/")
}

func MoveToFront(list []string, item string) []string {
	if len(list) == 0 || item == "" {
		return list
	}
	if list[0] == item {
		return list
	}

	ordered := make([]string, 0, len(list))
	ordered = append(ordered, item)

	for _, v := range list {
		if v != item {
			ordered = append(ordered, v)
		}
	}
	return ordered
}
