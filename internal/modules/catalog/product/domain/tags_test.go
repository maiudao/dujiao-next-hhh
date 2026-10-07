package productdomain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeProductTags(t *testing.T) {
	tests := []struct {
		name      string
		tags      []string
		published bool
		want      []string
		invalid   bool
	}{
		{"required", nil, true, nil, true},
		{"whitespace", []string{"  ", "\t"}, true, nil, true},
		{"unlisted", nil, false, []string{}, false},
		{"manual labels", []string{" 菲律宾区官方代充 ", "30天", "菲律宾区官方代充", ""}, true, []string{"菲律宾区官方代充", "30天"}, false},
		{"unicode boundary", []string{strings.Repeat("字", 40)}, true, []string{strings.Repeat("字", 40)}, false},
		{"too long", []string{strings.Repeat("字", 41)}, false, nil, true},
		{"too many", []string{"1", "2", "3", "4", "5", "6", "7"}, true, nil, true},
		{"multiline", []string{"a\nb"}, true, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeProductTags(tt.tags, tt.published)
			if errors.Is(err, ErrProductTagsInvalid) != tt.invalid || (!tt.invalid && !reflect.DeepEqual(got, tt.want)) {
				t.Fatalf("got %v, %v; want %v, invalid=%v", got, err, tt.want, tt.invalid)
			}
		})
	}
}
