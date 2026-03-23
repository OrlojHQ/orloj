package api

import "testing"

func TestParseResourceVersion(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"  ", 0},
		{"42", 42},
		{" 7 ", 7},
		{"-1", 0},
		{"notint", 0},
		{"9223372036854775807", 9223372036854775807},
	}
	for _, tt := range tests {
		if got := parseResourceVersion(tt.in); got != tt.want {
			t.Fatalf("parseResourceVersion(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseSinceID(t *testing.T) {
	tests := []struct {
		in   string
		want uint64
	}{
		{"", 0},
		{"0", 0},
		{"100", 100},
		{"  3  ", 3},
		{"bad", 0},
	}
	for _, tt := range tests {
		if got := parseSinceID(tt.in); got != tt.want {
			t.Fatalf("parseSinceID(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
