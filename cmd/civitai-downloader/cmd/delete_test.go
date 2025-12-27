package cmd

import (
	"testing"
)

func TestParseSelection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []int
		max      int
	}{
		{
			name:     "single number",
			input:    "1",
			max:      5,
			expected: []int{0}, // 0-based index
		},
		{
			name:     "multiple numbers comma separated",
			input:    "1,3,5",
			max:      5,
			expected: []int{0, 2, 4}, // 0-based indices
		},
		{
			name:     "range",
			input:    "1-3",
			max:      5,
			expected: []int{0, 1, 2}, // 0-based indices
		},
		{
			name:     "mixed range and numbers",
			input:    "1,3-5",
			max:      5,
			expected: []int{0, 2, 3, 4}, // 0-based indices
		},
		{
			name:     "all",
			input:    "all",
			max:      3,
			expected: []int{0, 1, 2},
		},
		{
			name:     "out of range ignored",
			input:    "1,10,100",
			max:      5,
			expected: []int{0}, // Only 1 is valid
		},
		{
			name:     "zero ignored",
			input:    "0,1,2",
			max:      5,
			expected: []int{0, 1}, // 0 is invalid (1-based input)
		},
		{
			name:     "negative ignored",
			input:    "-1,1,2",
			max:      5,
			expected: []int{0, 1},
		},
		{
			name:     "empty input",
			input:    "",
			max:      5,
			expected: []int{},
		},
		{
			name:     "spaces around numbers",
			input:    " 1 , 2 , 3 ",
			max:      5,
			expected: []int{0, 1, 2},
		},
		{
			name:     "duplicate numbers deduplicated",
			input:    "1,1,1,2",
			max:      5,
			expected: []int{0, 1},
		},
		{
			name:     "range with spaces",
			input:    "1 - 3",
			max:      5,
			expected: []int{0, 1, 2},
		},
		{
			name:     "invalid range ignored",
			input:    "5-2",
			max:      5,
			expected: []int{},
		},
		{
			name:     "partial overlap range",
			input:    "3-7",
			max:      5,
			expected: []int{}, // Range exceeds max, so entire range invalid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSelection(tt.input, tt.max)
			if !intSlicesEqual(got, tt.expected) {
				t.Errorf("parseSelection(%q, %d) = %v, want %v", tt.input, tt.max, got, tt.expected)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		maxLen   int
	}{
		{
			name:     "string shorter than max",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "string exactly max length",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "string longer than max",
			input:    "hello world",
			maxLen:   8,
			expected: "hello...",
		},
		{
			name:     "very short max length",
			input:    "hello",
			maxLen:   3,
			expected: "hel",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.expected {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
			}
		})
	}
}

// intSlicesEqual compares two int slices for equality
func intSlicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
