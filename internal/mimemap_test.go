package internal

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewMimeMap(t *testing.T) {
	mm1 := ""
	mm2 := "# ignore me"
	mm3 := "text/html text/plain\ntext/javascript text/plain"
	mm4 := "text/html text/plain\ntext/html text/duplicate"
	mm5 := "foo"
	mm6 := "foo bar buz"

	tests := []struct {
		mmText string
		valid  bool
	}{
		{mm1, true},
		{mm2, true},
		{mm3, true},
		{mm4, false},
		{mm5, false},
		{mm6, false},
	}

	for _, test := range tests {
		buff := bytes.NewBufferString(test.mmText)
		if _, err := NewMimeMap(buff); (err == nil) != test.valid {
			t.Fatalf("%s resulted in %v", test.mmText, err)
		}
	}
}

func TestMimeMap(t *testing.T) {
	mmBuff := bytes.NewBufferString(strings.TrimSpace(`
# Just a comment
text/html       text/plain
text/javascript text/plain

# such empty lines, wow

video/mp4	DROP
	`))

	mm, mmErr := NewMimeMap(mmBuff)
	if mmErr != nil {
		t.Fatal(mmErr)
	}

	tests := []struct {
		input  string
		output string
		valid  bool
		drop   bool
	}{
		{"text/html", "text/plain", true, false},
		{"text/javascript", "text/plain", true, false},
		{"video/mp4", "", false, true},
	}

	for _, test := range tests {
		if out, err := mm.Substitute(test.input); (err == nil) != test.valid {
			t.Fatalf("%s resulted in %v", test.input, err)
		} else if test.valid && out != test.output {
			t.Fatalf("%s mapped to %s, instead of %s", test.input, out, test.output)
		}

		if drop := mm.MustDrop(test.input); drop != test.drop {
			t.Fatalf("%s should be dropped: %t instead of %t", test.input, drop, test.drop)
		}
	}
}
