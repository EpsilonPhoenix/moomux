package terminal

import "fmt"

// fallbackOpener is used when no supported terminal is detected. It cannot
// open anything itself, so it hands the caller an attach instruction to
// display instead of writing to stdout directly — moomux runs its TUI on
// the alt screen, and writing straight to stdout there corrupts the display.
type fallbackOpener struct{}

func (f *fallbackOpener) OpenSession(tmuxSession, title string) (string, error) {
	// The "=" target is single-quoted: this line gets typed into an
	// interactive shell, and zsh's EQUALS expansion would otherwise read a
	// bare leading "=" as a command-path lookup and fail to find it.
	return fmt.Sprintf("no terminal detected, attach yourself: tmux attach -t '=%s'", tmuxSession), nil
}
