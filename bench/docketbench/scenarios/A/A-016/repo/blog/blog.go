package blog

import "strings"

// Post is one entry.
type Post struct {
	Title string
	Tags  []string
	Body  string
}

// Tagged reports whether p has tag.
func Tagged(p Post, tag string) bool {
	for _, t := range p.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Slug makes a url-safe title.
func Slug(title string) string {
	return strings.ToLower(strings.ReplaceAll(title, " ", "-"))
}
