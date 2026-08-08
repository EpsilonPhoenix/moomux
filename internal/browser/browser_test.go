package browser

import "testing"

// TestRemoteIgnoresStaleClientEnv covers a real false positive: on macOS,
// SSH_CLIENT/SSH_CONNECTION get cached into the per-user launchd environment
// after any SSH login and persist in local (non-SSH) terminal sessions too.
// Only SSH_TTY reliably reflects the current session.
func TestRemoteIgnoresStaleClientEnv(t *testing.T) {
	t.Setenv("SSH_TTY", "")
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_CLIENT", "100.91.153.105 59586 22")

	if Remote() {
		t.Errorf("Remote() = true with only stale SSH_CLIENT set, want false")
	}

	t.Setenv("SSH_TTY", "/dev/ttys002")
	if !Remote() {
		t.Errorf("Remote() = false with SSH_TTY set, want true")
	}
}

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
