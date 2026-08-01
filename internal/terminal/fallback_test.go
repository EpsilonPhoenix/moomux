package terminal

import (
	"strings"
	"testing"
)

func TestFallbackReturnsAttachHint(t *testing.T) {
	f := &fallbackOpener{}
	hint, err := f.OpenSession("moomux-foo", "feat/bar")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hint, `tmux attach -t '=moomux-foo'`) {
		t.Fatalf("expected attach command in hint, got: %s", hint)
	}
}

func TestFallbackUsesProcessTreeInsideTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	withAttachedClient(t, []string{"zsh", "login", "iTerm2"})

	got := fallback()
	if _, ok := got.(*itermClient); !ok {
		t.Fatalf("expected *itermClient, got %T", got)
	}
}

func TestFallbackReturnsFallbackOpenerWhenProcessTreeUnknownInsideTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	withAttachedClient(t, []string{"zsh", "sshd"})

	if _, ok := fallback().(*fallbackOpener); !ok {
		t.Fatalf("expected *fallbackOpener, got %T", fallback())
	}
}

func TestFallbackFuncReturnsFallbackOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	if _, ok := fallback().(*fallbackOpener); !ok {
		t.Fatalf("expected *fallbackOpener, got %T", fallback())
	}
}
