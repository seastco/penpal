package client

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Terminal dimensions — updated on each tea.WindowSizeMsg
	termWidth  = 80
	termHeight = 24

	// Adaptive color palette — reassigned by applyTheme()
	colorPrimary lipgloss.AdaptiveColor
	colorAccent  lipgloss.AdaptiveColor
	colorMuted   lipgloss.AdaptiveColor
	colorBorder  lipgloss.AdaptiveColor
	colorText    lipgloss.AdaptiveColor
	colorSuccess lipgloss.AdaptiveColor
	colorError   lipgloss.AdaptiveColor
	colorNew     lipgloss.AdaptiveColor
	colorDim     lipgloss.AdaptiveColor

	// Styles — rebuilt by applyTheme()
	titleStyle    lipgloss.Style
	dividerStyle  lipgloss.Style
	menuStyle     lipgloss.Style
	menuKeyStyle  lipgloss.Style
	selectedStyle lipgloss.Style
	mutedStyle    lipgloss.Style
	newStyle      lipgloss.Style
	errorStyle    lipgloss.Style
	successStyle  lipgloss.Style
	helpStyle     lipgloss.Style
	dimStyle      lipgloss.Style
)

// Theme defines a color palette with dark and light variants.
type Theme struct {
	Name    string
	Primary lipgloss.AdaptiveColor
	Accent  lipgloss.AdaptiveColor
	Muted   lipgloss.AdaptiveColor
	Border  lipgloss.AdaptiveColor
	Text    lipgloss.AdaptiveColor
	Success lipgloss.AdaptiveColor
	Error   lipgloss.AdaptiveColor
	New     lipgloss.AdaptiveColor
	Dim     lipgloss.AdaptiveColor
}

var themes = []Theme{
	{
		Name:    "Catppuccin",
		Primary: lipgloss.AdaptiveColor{Dark: "#89B4FA", Light: "#1E66F5"},
		Accent:  lipgloss.AdaptiveColor{Dark: "#74C7EC", Light: "#209FB5"},
		Muted:   lipgloss.AdaptiveColor{Dark: "#6C7086", Light: "#9CA0B0"},
		Border:  lipgloss.AdaptiveColor{Dark: "#45475A", Light: "#BCC0CC"},
		Text:    lipgloss.AdaptiveColor{Dark: "#CDD6F4", Light: "#4C4F69"},
		Success: lipgloss.AdaptiveColor{Dark: "#A6E3A1", Light: "#40A02B"},
		Error:   lipgloss.AdaptiveColor{Dark: "#F38BA8", Light: "#D20F39"},
		New:     lipgloss.AdaptiveColor{Dark: "#FAB387", Light: "#FE640B"},
		Dim:     lipgloss.AdaptiveColor{Dark: "#585B70", Light: "#CCD0DA"},
	},
	{
		Name:    "Nord",
		Primary: lipgloss.AdaptiveColor{Dark: "#88C0D0", Light: "#5E81AC"},
		Accent:  lipgloss.AdaptiveColor{Dark: "#81A1C1", Light: "#5E81AC"},
		Muted:   lipgloss.AdaptiveColor{Dark: "#4C566A", Light: "#9099AB"},
		Border:  lipgloss.AdaptiveColor{Dark: "#3B4252", Light: "#D8DEE9"},
		Text:    lipgloss.AdaptiveColor{Dark: "#ECEFF4", Light: "#2E3440"},
		Success: lipgloss.AdaptiveColor{Dark: "#A3BE8C", Light: "#4C8C5E"},
		Error:   lipgloss.AdaptiveColor{Dark: "#BF616A", Light: "#BF616A"},
		New:     lipgloss.AdaptiveColor{Dark: "#D08770", Light: "#D08770"},
		Dim:     lipgloss.AdaptiveColor{Dark: "#434C5E", Light: "#D8DEE9"},
	},
	{
		Name:    "Rosé Pine",
		Primary: lipgloss.AdaptiveColor{Dark: "#C4A7E7", Light: "#907AA9"},
		Accent:  lipgloss.AdaptiveColor{Dark: "#EBBCBA", Light: "#D7827E"},
		Muted:   lipgloss.AdaptiveColor{Dark: "#6E6A86", Light: "#9893A5"},
		Border:  lipgloss.AdaptiveColor{Dark: "#403D52", Light: "#DFDAD9"},
		Text:    lipgloss.AdaptiveColor{Dark: "#E0DEF4", Light: "#575279"},
		Success: lipgloss.AdaptiveColor{Dark: "#9CCFD8", Light: "#56949F"},
		Error:   lipgloss.AdaptiveColor{Dark: "#EB6F92", Light: "#B4637A"},
		New:     lipgloss.AdaptiveColor{Dark: "#F6C177", Light: "#EA9D34"},
		Dim:     lipgloss.AdaptiveColor{Dark: "#524F67", Light: "#E4DFDE"},
	},
	{
		Name:    "Gruvbox",
		Primary: lipgloss.AdaptiveColor{Dark: "#83A598", Light: "#427B58"},
		Accent:  lipgloss.AdaptiveColor{Dark: "#FABD2F", Light: "#B57614"},
		Muted:   lipgloss.AdaptiveColor{Dark: "#665C54", Light: "#A89984"},
		Border:  lipgloss.AdaptiveColor{Dark: "#504945", Light: "#D5C4A1"},
		Text:    lipgloss.AdaptiveColor{Dark: "#EBDBB2", Light: "#3C3836"},
		Success: lipgloss.AdaptiveColor{Dark: "#B8BB26", Light: "#79740E"},
		Error:   lipgloss.AdaptiveColor{Dark: "#FB4934", Light: "#CC241D"},
		New:     lipgloss.AdaptiveColor{Dark: "#FE8019", Light: "#AF3A03"},
		Dim:     lipgloss.AdaptiveColor{Dark: "#3C3836", Light: "#D5C4A1"},
	},
	{
		Name:    "Dracula",
		Primary: lipgloss.AdaptiveColor{Dark: "#BD93F9", Light: "#7C3AED"},
		Accent:  lipgloss.AdaptiveColor{Dark: "#8BE9FD", Light: "#0891B2"},
		Muted:   lipgloss.AdaptiveColor{Dark: "#6272A4", Light: "#94A3B8"},
		Border:  lipgloss.AdaptiveColor{Dark: "#44475A", Light: "#CBD5E1"},
		Text:    lipgloss.AdaptiveColor{Dark: "#F8F8F2", Light: "#1E293B"},
		Success: lipgloss.AdaptiveColor{Dark: "#50FA7B", Light: "#16A34A"},
		Error:   lipgloss.AdaptiveColor{Dark: "#FF5555", Light: "#DC2626"},
		New:     lipgloss.AdaptiveColor{Dark: "#FFB86C", Light: "#EA580C"},
		Dim:     lipgloss.AdaptiveColor{Dark: "#44475A", Light: "#CBD5E1"},
	},
}

func init() {
	applyTheme(themes[0]) // default to Catppuccin
}

// applyTheme sets all color and style variables from the given theme.
func applyTheme(t Theme) {
	colorPrimary = t.Primary
	colorAccent = t.Accent
	colorMuted = t.Muted
	colorBorder = t.Border
	colorText = t.Text
	colorSuccess = t.Success
	colorError = t.Error
	colorNew = t.New
	colorDim = t.Dim

	titleStyle = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	dividerStyle = lipgloss.NewStyle().Foreground(colorBorder)
	menuStyle = lipgloss.NewStyle().Foreground(colorText)
	menuKeyStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)
	newStyle = lipgloss.NewStyle().Foreground(colorNew).Bold(true)
	errorStyle = lipgloss.NewStyle().Foreground(colorError)
	successStyle = lipgloss.NewStyle().Foreground(colorSuccess)
	helpStyle = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	dimStyle = lipgloss.NewStyle().Foreground(colorDim)
}

// themeByName returns the theme with the given name, or the default theme.
func themeByName(name string) Theme {
	for _, t := range themes {
		if strings.EqualFold(t.Name, name) {
			return t
		}
	}
	return themes[0]
}

// loadTheme reads the saved theme from ~/.penpal/theme and applies it.
func loadTheme(homeDir string) string {
	data, err := os.ReadFile(filepath.Join(homeDir, "theme"))
	if err != nil {
		return themes[0].Name
	}
	name := strings.TrimSpace(string(data))
	t := themeByName(name)
	applyTheme(t)
	return t.Name
}

// saveTheme writes the theme name to ~/.penpal/theme.
func saveTheme(homeDir, name string) error {
	return os.WriteFile(filepath.Join(homeDir, "theme"), []byte(name+"\n"), 0644)
}

const (
	minBoxWidth     = 50
	maxBoxWidth     = 76
	stampCardInnerW = 10
	stampsPerRow    = 5
)

// boxWidth returns the responsive outer box width.
func boxWidth() int {
	w := termWidth - 2
	if w < minBoxWidth {
		w = minBoxWidth
	}
	if w > maxBoxWidth {
		w = maxBoxWidth
	}
	return w
}

// contentWidth returns the usable width inside the box (minus border + padding).
func contentWidth() int {
	return boxWidth() - 6 // 2 border + 4 padding
}

// screenBox returns a responsive styled box.
func screenBox() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 2).
		Width(boxWidth())
}

// centeredView places content in the center of the terminal.
func centeredView(content string) string {
	return lipgloss.Place(termWidth, termHeight,
		lipgloss.Center, lipgloss.Center,
		content)
}

// headerLine renders a left-aligned and right-aligned string on one line,
// filling the gap with spaces to span the full content width.
func headerLine(left, right string) string {
	gap := contentWidth() - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		gap = 2
	}
	return left + strings.Repeat(" ", gap) + right
}

func divider(width int) string {
	return dividerStyle.Render(strings.Repeat("─", width))
}

// boxHeight returns the responsive outer box height for scrollable screens.
func boxHeight() int {
	h := termHeight - 2
	if h < 14 {
		h = 14
	}
	return h
}

// viewportHeight returns the scrollable area height inside a fixed box.
// Subtracts: padding(2) + header(2) + footer(2)
func viewportHeight() int {
	h := boxHeight() - 6
	if h < 4 {
		h = 4
	}
	return h
}

// screenBoxFixed returns a responsive styled box with fixed height for scrollable screens.
func screenBoxFixed() lipgloss.Style {
	return screenBox().Height(boxHeight())
}
