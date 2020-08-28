package internal

import (
	"math"
	"testing"
	"time"
)

func TestDurationPattern(t *testing.T) {
	durationPattern = nil
	_ = getDurationPattern()

	if durationPattern == nil {
		t.Fatalf("durationPattern is still nil")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input  string
		output time.Duration
		valid  bool
	}{
		{"1d5m", 24*time.Hour + 5*time.Minute, true},
		{"4w", 4 * 7 * 24 * time.Hour, true},
		{"1m10h", 0, false},
		{"", 0, false},
		{"-1m", 0, false},
	}

	for _, test := range tests {
		d, err := ParseDuration(test.input)
		if (err == nil) != test.valid {
			t.Fatalf("Error: %v, expected %t", err, test.valid)
		}

		if !test.valid {
			continue
		}

		if d != test.output {
			t.Fatalf("Duration mismatches, expected %v and got %v", test.output, d)
		}
	}
}

func TestPrettyDuration(t *testing.T) {
	tests := []struct {
		input  time.Duration
		output string
	}{
		{time.Minute, "1 minute"},
		{5 * time.Minute, "5 minutes"},
		{2*time.Hour + 4*time.Minute + 10*time.Second, "2 hours 4 minutes 10 seconds"},
		{timeYear, "1 year"},
		{12 * timeMonth, "1 year"},
		{13 * timeMonth, "1 year 1 month"},
	}

	for _, test := range tests {
		if pretty := PrettyDuration(test.input); pretty != test.output {
			t.Fatalf("%v resulted in %s instead of %s", test.input, pretty, test.output)
		}
	}
}

func TestParseBytesize(t *testing.T) {
	tests := []struct {
		input  string
		output int64
		valid  bool
	}{
		{"1B", 1, true},
		{"1MB", 1024 * 1024, true},
		{"1MiB", 1024 * 1024, true},
		{"23KiB", 23 * 1024, true},
		{"1Mi", 0, false},
		{"0B", 0, false},
		{"", 0, false},
	}

	for _, test := range tests {
		bs, err := ParseBytesize(test.input)
		if (err == nil) != test.valid {
			t.Fatalf("Error: %v, expected %t", err, test.valid)
		}

		if !test.valid {
			continue
		}

		if bs != test.output {
			t.Fatalf("Size mismatches, expected %d and got %d", test.output, bs)
		}
	}
}

func TestPrettyBytesize(t *testing.T) {
	tests := []struct {
		input  int64
		output string
	}{
		{1, "1.0 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1536 * 1024, "1.5 MiB"},
		{1536 * 1024 * 1024, "1.5 GiB"},
		{math.MaxInt64, "too big to fail"},
	}

	for _, test := range tests {
		if pretty := PrettyBytesize(test.input); pretty != test.output {
			t.Fatalf("%d resulted in %s instead of %s", test.input, pretty, test.output)
		}
	}
}
