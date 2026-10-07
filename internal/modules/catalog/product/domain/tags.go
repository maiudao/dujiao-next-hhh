package productdomain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var ErrProductTagsInvalid = errors.New("product tags invalid")

// NormalizeProductTags keeps manually entered labels consistent across all publish paths.
// Unlisted products may be saved without labels; published products need at least one.
func NormalizeProductTags(tags []string, published bool) ([]string, error) {
	result := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		if tag == "" || seen[tag] {
			continue
		}
		if utf8.RuneCountInString(tag) > 40 || strings.ContainsAny(tag, "\r\n\t") {
			return nil, ErrProductTagsInvalid
		}
		seen[tag] = true
		result = append(result, tag)
	}
	if len(result) > 6 || (published && len(result) == 0) {
		return nil, ErrProductTagsInvalid
	}
	return result, nil
}
