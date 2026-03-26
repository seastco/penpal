package client

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	// Terminal dimensions — updated on each tea.WindowSizeMsg
	termWidth  = 80
	termHeight = 24

	// Resolved color palette — reassigned by applyTheme()
	colorPrimary color.Color
	colorAccent  color.Color
	colorMuted   color.Color
	colorBorder  color.Color
	colorText    color.Color
	colorSuccess color.Color
	colorError   color.Color
	colorNew     color.Color
	colorDim     color.Color

	// isDark tracks whether the terminal has a dark background.
	isDark       = true
	currentTheme Theme

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

// colorPair holds dark and light color variants.
type colorPair struct {
	Dark  color.Color
	Light color.Color
}

// Theme defines a color palette with dark and light variants.
type Theme struct {
	Name    string
	Primary colorPair
	Accent  colorPair
	Muted   colorPair
	Border  colorPair
	Text    colorPair
	Success colorPair
	Error   colorPair
	New     colorPair
	Dim     colorPair
}

// ac is a shorthand for creating color pairs from hex strings.
func ac(dark, light string) colorPair {
	return colorPair{Dark: lipgloss.Color(dark), Light: lipgloss.Color(light)}
}

// pickColor resolves a colorPair to a single color based on isDark.
func pickColor(cp colorPair) color.Color {
	if isDark {
		return cp.Dark
	}
	return cp.Light
}

var themes = []Theme{
	{
		Name:    "Catppuccin",
		Primary: ac("#89B4FA", "#1E66F5"),
		Accent:  ac("#74C7EC", "#209FB5"),
		Muted:   ac("#6C7086", "#9CA0B0"),
		Border:  ac("#45475A", "#BCC0CC"),
		Text:    ac("#CDD6F4", "#4C4F69"),
		Success: ac("#A6E3A1", "#40A02B"),
		Error:   ac("#F38BA8", "#D20F39"),
		New:     ac("#FAB387", "#FE640B"),
		Dim:     ac("#585B70", "#CCD0DA"),
	},
	{
		Name:    "Nord",
		Primary: ac("#88C0D0", "#5E81AC"),
		Accent:  ac("#81A1C1", "#5E81AC"),
		Muted:   ac("#4C566A", "#9099AB"),
		Border:  ac("#3B4252", "#D8DEE9"),
		Text:    ac("#ECEFF4", "#2E3440"),
		Success: ac("#A3BE8C", "#4C8C5E"),
		Error:   ac("#BF616A", "#BF616A"),
		New:     ac("#D08770", "#D08770"),
		Dim:     ac("#434C5E", "#D8DEE9"),
	},
	{
		Name:    "Rosé Pine",
		Primary: ac("#C4A7E7", "#907AA9"),
		Accent:  ac("#EBBCBA", "#D7827E"),
		Muted:   ac("#6E6A86", "#9893A5"),
		Border:  ac("#403D52", "#DFDAD9"),
		Text:    ac("#E0DEF4", "#575279"),
		Success: ac("#9CCFD8", "#56949F"),
		Error:   ac("#EB6F92", "#B4637A"),
		New:     ac("#F6C177", "#EA9D34"),
		Dim:     ac("#524F67", "#E4DFDE"),
	},
	{
		Name:    "Gruvbox",
		Primary: ac("#83A598", "#427B58"),
		Accent:  ac("#FABD2F", "#B57614"),
		Muted:   ac("#665C54", "#A89984"),
		Border:  ac("#504945", "#D5C4A1"),
		Text:    ac("#EBDBB2", "#3C3836"),
		Success: ac("#B8BB26", "#79740E"),
		Error:   ac("#FB4934", "#CC241D"),
		New:     ac("#FE8019", "#AF3A03"),
		Dim:     ac("#3C3836", "#D5C4A1"),
	},
	{
		Name:    "Dracula",
		Primary: ac("#BD93F9", "#7C3AED"),
		Accent:  ac("#8BE9FD", "#0891B2"),
		Muted:   ac("#6272A4", "#94A3B8"),
		Border:  ac("#44475A", "#CBD5E1"),
		Text:    ac("#F8F8F2", "#1E293B"),
		Success: ac("#50FA7B", "#16A34A"),
		Error:   ac("#FF5555", "#DC2626"),
		New:     ac("#FFB86C", "#EA580C"),
		Dim:     ac("#44475A", "#CBD5E1"),
	},
}

func init() {
	applyTheme(themes[0]) // default to Catppuccin
}

// applyTheme sets all color and style variables from the given theme.
func applyTheme(t Theme) {
	currentTheme = t

	colorPrimary = pickColor(t.Primary)
	colorAccent = pickColor(t.Accent)
	colorMuted = pickColor(t.Muted)
	colorBorder = pickColor(t.Border)
	colorText = pickColor(t.Text)
	colorSuccess = pickColor(t.Success)
	colorError = pickColor(t.Error)
	colorNew = pickColor(t.New)
	colorDim = pickColor(t.Dim)

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

// setDarkMode updates the dark/light preference and reapplies the current theme.
func setDarkMode(dark bool) {
	isDark = dark
	applyTheme(currentTheme)
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
		Width(boxWidth() + 2) // +2 because lipgloss v2 includes border in width
}

// screenBoxEmpty returns a box that matches the home screen's height,
// used for empty-state screens so the box size stays consistent.
func screenBoxEmpty() lipgloss.Style {
	return screenBox().Height(13)
}

// emptyScreenView renders an empty-state screen with footer bottom-aligned.
func emptyScreenView(header, body, footerText string, indentHelp ...bool) string {
	top := header + body
	topLines := lipgloss.Height(top)
	available := 13 - 2 // usable content lines (Height minus top+bottom padding)
	gap := available - topLines
	if gap < 1 {
		gap = 1
	}
	indent := ""
	if len(indentHelp) > 0 && indentHelp[0] {
		indent = "  "
	}
	content := top + strings.Repeat("\n", gap) + indent + helpStyle.Render(footerText)
	return screenBoxEmpty().Render(content)
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
// Subtracts 4 (not 2) so the rendered box (Height + 2 for borders) leaves
// centering margin in centeredView.
func boxHeight() int {
	h := termHeight - 4
	if h < 14 {
		h = 14
	}
	return h
}

// adaptiveBoxHeight returns a box height that stays compact for small content
// but grows up to boxHeight() when scrolling is needed.
// contentLines = number of lines the viewport content occupies.
// overhead = header + footer + padding lines (everything except viewport).
func adaptiveBoxHeight(contentLines, overhead int) int {
	h := contentLines + overhead
	if h < 13 {
		h = 13
	}
	if maxH := boxHeight(); h > maxH {
		h = maxH
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
