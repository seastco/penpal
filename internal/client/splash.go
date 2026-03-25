package client

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type splashTickMsg time.Time

// SplashModel shows an animated typewriter splash with a sliding ✉ emoji.
type SplashModel struct {
	app   *AppState
	frame int
}

func NewSplashModel(app *AppState) SplashModel {
	return SplashModel{app: app}
}

const splashTotalFrames = 10

func (m SplashModel) Init() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(t time.Time) tea.Msg { return splashTickMsg(t) })
}

func (m SplashModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyPressMsg:
		return m, func() tea.Msg { return switchScreenMsg{screen: ScreenHome} }
	case splashTickMsg:
		m.frame++
		if m.frame >= splashTotalFrames {
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenHome} }
		}
		return m, tea.Tick(400*time.Millisecond, func(t time.Time) tea.Msg { return splashTickMsg(t) })
	}
	return m, nil
}

func (m SplashModel) View() tea.View {
	f := m.frame

	titleSt := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	tagSt := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	envSt := lipgloss.NewStyle().Foreground(colorAccent)

	const (
		fullText = "P E N P A L"
		tagText  = "letters that take their time"
		sw       = 28 // splash width = len(tagText), prevents centering jitter
	)
	letterPos := []int{0, 2, 4, 6, 8, 10}
	titleLeft := (sw - len(fullText)) / 2 // 8

	// Pad line to sw visual columns to prevent centering jitter between frames.
	padTo := func(s string, visW int) string {
		if visW < sw {
			return s + strings.Repeat(" ", sw-visW)
		}
		return s
	}
	blank := strings.Repeat(" ", sw)

	var b strings.Builder

	// Title is always in the same position (line 1 of 5).
	// During typing, ✉ slides under each letter on line 2.
	// After typing, tagline and centered ✉ appear below.

	tLine := strings.Repeat(" ", titleLeft) + titleSt.Render(fullText)
	tVisW := titleLeft + len(fullText)

	switch {
	case f < 1:
		// Brief pause
		b.WriteString(blank + "\n")
		b.WriteString(blank + "\n")
		b.WriteString(blank + "\n")
		b.WriteString(blank + "\n")
		b.WriteString(blank + "\n")

	case f < 7:
		// Typing: title fixed, ✉ slides under each new letter
		idx := f - 1 // 0..5
		typed := fullText[:letterPos[idx]+1]
		typedLine := strings.Repeat(" ", titleLeft) + titleSt.Render(typed)
		typedVisW := titleLeft + len(typed)

		eCol := titleLeft + letterPos[idx]
		eLine := strings.Repeat(" ", eCol) + envSt.Render("✉")
		eVisW := eCol + 2

		b.WriteString(padTo(typedLine, typedVisW) + "\n")
		b.WriteString(padTo(eLine, eVisW) + "\n")
		b.WriteString(blank + "\n")
		b.WriteString(blank + "\n")
		b.WriteString(blank + "\n")

	default:
		// Tagline with ✉ centered below
		eCol := (sw - 2) / 2
		eLine := strings.Repeat(" ", eCol) + envSt.Render("✉")
		eVisW := eCol + 2

		b.WriteString(padTo(tLine, tVisW) + "\n")
		b.WriteString(blank + "\n")
		b.WriteString(tagSt.Render(tagText) + "\n")
		b.WriteString(blank + "\n")
		b.WriteString(padTo(eLine, eVisW) + "\n")
	}

	// Match the home screen box exactly: 10 content lines inside screenBox().
	inner := lipgloss.NewStyle().
		Width(contentWidth()).
		Height(10).
		AlignVertical(lipgloss.Center).
		AlignHorizontal(lipgloss.Center).
		Render(b.String())
	return tea.NewView(screenBox().Render(inner))
}
