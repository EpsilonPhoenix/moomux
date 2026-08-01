package browser

import "testing"

func TestOpenRejectsNonHTTPURLs(t *testing.T) {
	cases := []string{
		"",
		"-a",
		"-a Safari",
		"file:///tmp/notes.txt",
		"javascript:alert(1)",
		"ftp://example.com/x",
		"http://",
	}
	for _, raw := range cases {
		if err := Open(raw); err == nil {
			t.Errorf("Open(%q): expected error, got nil", raw)
		}
	}
}
