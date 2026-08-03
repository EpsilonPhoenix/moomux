package tui

import (
	"sync"

	"github.com/charmbracelet/lipgloss"

	"github.com/erickgnclvs/moomux/internal/config"
)

// ApplySettings applies a loaded config's saved theme/appearance before the
// TUI's first render — main wires this in ahead of tea.NewProgram.
func ApplySettings(cfg *config.Config) {
	applyAppearance(cfg.Appearance)
	applyTheme(cfg.Theme)
}

// palette is the full set of colors a theme swaps in. Every field mirrors one
// of the package-level colXxx vars in styles.go.
type palette struct {
	fg, mute, accent, working, done, needsInput, parked, danger, border, selBg lipgloss.AdaptiveColor
}

// themes are the built-in palettes. "default" is moomux's original look;
// "terminal" delegates to the user's own terminal colorscheme via ANSI
// indices, which is why its Light/Dark halves match — the terminal itself
// already resolves those per its own appearance.
var themes = map[string]palette{
	"default": {
		fg:         lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#e6e6e6"},
		mute:       lipgloss.AdaptiveColor{Light: "#5b5b66", Dark: "#7a7a85"},
		accent:     lipgloss.AdaptiveColor{Light: "#2952cc", Dark: "#7aa2f7"},
		working:    lipgloss.AdaptiveColor{Light: "#4b7a1f", Dark: "#9ece6a"},
		done:       lipgloss.AdaptiveColor{Light: "#946f1a", Dark: "#e0af68"},
		needsInput: lipgloss.AdaptiveColor{Light: "#9c3ba1", Dark: "#bb70d2"},
		parked:     lipgloss.AdaptiveColor{Light: "#7d7d85", Dark: "#565a6e"},
		danger:     lipgloss.AdaptiveColor{Light: "#c0293f", Dark: "#f7768e"},
		border:     lipgloss.AdaptiveColor{Light: "#9a9aa5", Dark: "#2d2f3a"},
		selBg:      lipgloss.AdaptiveColor{Light: "#a9bdf0", Dark: "#2f395e"},
	},
	"terminal": {
		fg:         lipgloss.AdaptiveColor{Light: "15", Dark: "15"},
		mute:       lipgloss.AdaptiveColor{Light: "8", Dark: "8"},
		accent:     lipgloss.AdaptiveColor{Light: "12", Dark: "12"},
		working:    lipgloss.AdaptiveColor{Light: "10", Dark: "10"},
		done:       lipgloss.AdaptiveColor{Light: "11", Dark: "11"},
		needsInput: lipgloss.AdaptiveColor{Light: "13", Dark: "13"},
		parked:     lipgloss.AdaptiveColor{Light: "7", Dark: "7"},
		danger:     lipgloss.AdaptiveColor{Light: "9", Dark: "9"},
		border:     lipgloss.AdaptiveColor{Light: "8", Dark: "8"},
		selBg:      lipgloss.AdaptiveColor{Light: "4", Dark: "4"},
	},
	"gruvbox": {
		fg:         lipgloss.AdaptiveColor{Light: "#3c3836", Dark: "#ebdbb2"},
		mute:       lipgloss.AdaptiveColor{Light: "#665c54", Dark: "#a89984"},
		accent:     lipgloss.AdaptiveColor{Light: "#076678", Dark: "#83a598"},
		working:    lipgloss.AdaptiveColor{Light: "#79740e", Dark: "#b8bb26"},
		done:       lipgloss.AdaptiveColor{Light: "#b57614", Dark: "#fabd2f"},
		needsInput: lipgloss.AdaptiveColor{Light: "#8f3f71", Dark: "#d3869b"},
		parked:     lipgloss.AdaptiveColor{Light: "#a89984", Dark: "#665c54"},
		danger:     lipgloss.AdaptiveColor{Light: "#9d0006", Dark: "#fb4934"},
		border:     lipgloss.AdaptiveColor{Light: "#d5c4a1", Dark: "#3c3836"},
		selBg:      lipgloss.AdaptiveColor{Light: "#d5c4a1", Dark: "#504945"},
	},
	"catppuccin": {
		fg:         lipgloss.AdaptiveColor{Light: "#4c4f69", Dark: "#cdd6f4"},
		mute:       lipgloss.AdaptiveColor{Light: "#6c6f85", Dark: "#a6adc8"},
		accent:     lipgloss.AdaptiveColor{Light: "#1e66f5", Dark: "#89b4fa"},
		working:    lipgloss.AdaptiveColor{Light: "#40a02b", Dark: "#a6e3a1"},
		done:       lipgloss.AdaptiveColor{Light: "#df8e1d", Dark: "#f9e2af"},
		needsInput: lipgloss.AdaptiveColor{Light: "#8839ef", Dark: "#cba6f7"},
		parked:     lipgloss.AdaptiveColor{Light: "#9ca0b0", Dark: "#6c7086"},
		danger:     lipgloss.AdaptiveColor{Light: "#d20f39", Dark: "#f38ba8"},
		border:     lipgloss.AdaptiveColor{Light: "#acb0be", Dark: "#585b70"},
		selBg:      lipgloss.AdaptiveColor{Light: "#ccd0da", Dark: "#313244"},
	},
}

// themeNames is the stable display/cycle order for the theme picker — map
// iteration order isn't stable, and "default" belongs first regardless.
var themeNames = []string{"default", "terminal", "gruvbox", "catppuccin"}

// themeIndex returns name's position in themeNames, or 0 ("default") if name
// is empty or unrecognized (e.g. an unset or hand-edited config field).
func themeIndex(name string) int {
	for i, n := range themeNames {
		if n == name {
			return i
		}
	}
	return 0
}

// applyTheme swaps the active color palette and rebuilds every style and
// pre-rendered string derived from it. Unknown names fall back to "default"
// rather than erroring, since this also runs against whatever a hand-edited
// config.toml contains.
func applyTheme(name string) {
	p, ok := themes[name]
	if !ok {
		p = themes["default"]
	}
	colFg = p.fg
	colMute = p.mute
	colAccent = p.accent
	colWorking = p.working
	colDone = p.done
	colNeedsInput = p.needsInput
	colParked = p.parked
	colDanger = p.danger
	colBorder = p.border
	colSelBg = p.selBg
	buildStyles()
}

// autoDark lazily detects and caches the terminal's actual background once,
// the first time "auto" appearance is applied — not at package init, so
// tests and non-interactive runs that never touch appearance don't pay for
// an OSC-11 terminal query. lipgloss.SetHasDarkBackground latches its value
// permanently once called, so restoring "auto" after an explicit override
// requires remembering the real detected value ourselves.
var (
	autoDarkOnce sync.Once
	autoDarkVal  bool
)

func autoDark() bool {
	autoDarkOnce.Do(func() {
		autoDarkVal = lipgloss.HasDarkBackground()
	})
	return autoDarkVal
}

// applyAppearance overrides (or restores) which half of every AdaptiveColor
// pair renders. "light"/"dark" force it; anything else (including "", the
// zero-value config default) restores auto-detection.
func applyAppearance(mode string) {
	switch mode {
	case "light":
		lipgloss.SetHasDarkBackground(false)
	case "dark":
		lipgloss.SetHasDarkBackground(true)
	default:
		lipgloss.SetHasDarkBackground(autoDark())
	}
}

// nextAppearance cycles auto ("") -> light -> dark -> auto, the order the
// theme picker's 'a' key steps through.
func nextAppearance(cur string) string {
	switch cur {
	case "":
		return "light"
	case "light":
		return "dark"
	default:
		return ""
	}
}

// appearanceLabel renders an appearance value for display, since the
// zero-value "" is displayed as "auto" rather than left blank.
func appearanceLabel(mode string) string {
	if mode == "" {
		return "auto"
	}
	return mode
}
