package hashtag_test

import (
	"reflect"
	"testing"

	"github.com/twitter/tweet-service/internal/hashtag"
)

func TestExtract(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single hashtag",
			input: "Hello #world",
			want:  []string{"#world"},
		},
		{
			name:  "multiple hashtags",
			input: "Loving #Go and #microservices today",
			want:  []string{"#go", "#microservices"},
		},
		{
			name:  "no hashtags",
			input: "Just a plain tweet",
			want:  []string{},
		},
		{
			name:  "empty string",
			input: "",
			want:  []string{},
		},
		{
			name:  "deduplicates case-insensitive",
			input: "#Go is great, love #go and #GO",
			want:  []string{"#go"},
		},
		{
			name:  "lowercases output",
			input: "#GoLang #GOLANG",
			want:  []string{"#golang"},
		},
		{
			name:  "hashtag at start",
			input: "#trending tweet right here",
			want:  []string{"#trending"},
		},
		{
			name:  "hashtag with numbers",
			input: "loving #go1 today",
			want:  []string{"#go1"},
		},
		{
			name:  "hash without word is ignored",
			input: "price is 100# off",
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hashtag.Extract(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Extract(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtract_ReturnsNonNilSlice(t *testing.T) {
	got := hashtag.Extract("no hashtags here")
	if got == nil {
		t.Error("Extract returned nil, want empty non-nil slice")
	}
}
