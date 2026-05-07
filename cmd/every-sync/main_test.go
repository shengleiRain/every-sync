package main

import "testing"

func TestParseByteSize(t *testing.T) {
	tests := map[string]int64{
		"":      0,
		"0":     0,
		"512":   512,
		"1KB":   1024,
		"1.5MB": 1572864,
		"2GB":   2147483648,
		"8MiB":  8388608,
	}
	for input, want := range tests {
		got, err := parseByteSize(input)
		if err != nil {
			t.Fatalf("parseByteSize(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("parseByteSize(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestParseByteRate(t *testing.T) {
	tests := map[string]int64{
		"0":      0,
		"10MB/s": 10485760,
		"2KBps":  2048,
	}
	for input, want := range tests {
		got, err := parseByteRate(input)
		if err != nil {
			t.Fatalf("parseByteRate(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("parseByteRate(%q) = %d, want %d", input, got, want)
		}
	}
}
