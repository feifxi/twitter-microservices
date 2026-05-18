package hashtag

import (
	"regexp"
	"strings"
)

var hashtagRe = regexp.MustCompile(`#\w+`)

func Extract(text string) []string {
	matches := hashtagRe.FindAllString(text, -1)
	if len(matches) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		tag := strings.ToLower(m)
		if _, dup := seen[tag]; !dup {
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	return out
}
