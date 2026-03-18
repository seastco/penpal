package client

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	pencrypto "github.com/seastco/penpal/internal/crypto"
	"golang.org/x/crypto/bcrypt"
)

// --- PIN storage ---

func pinFilePath() (string, error) {
	dir, err := pencrypto.PenpalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pin"), nil
}

// clearPin removes any existing PIN file.
func clearPin() {
	p, err := pinFilePath()
	if err != nil {
		return
	}
	os.Remove(p)
}

// PinFileExists checks whether a PIN hash has been saved.
func PinFileExists() bool {
	p, err := pinFilePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// SavePinHash bcrypt-hashes the PIN and writes it to ~/.penpal/pin.
func SavePinHash(pin string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hashing pin: %w", err)
	}
	p, err := pinFilePath()
	if err != nil {
		return err
	}
	return os.WriteFile(p, hash, 0600)
}

// VerifyPin checks a PIN against the stored hash.
func VerifyPin(pin string) bool {
	p, err := pinFilePath()
	if err != nil {
		return false
	}
	hash, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword(hash, []byte(pin)) == nil
}

// --- Shared dot renderer ---

func renderDots(filled, total int, shaking bool) string {
	var parts []string
	for i := 0; i < total; i++ {
		if i < filled {
			if shaking {
				parts = append(parts, errorStyle.Render("●"))
			} else {
				parts = append(parts, selectedStyle.Render("●"))
			}
		} else {
			parts = append(parts, mutedStyle.Render("○"))
		}
	}
	return strings.Join(parts, " ")
}

// --- PIN Entry (returning users with a PIN) ---

type pinShakeTickMsg struct{}

type PinEntryModel struct {
	app       *AppState
	digits    []rune
	shaking   bool
	shakeTick int
}

func NewPinEntryModel(app *AppState) PinEntryModel {
	return PinEntryModel{app: app}
}

func (m PinEntryModel) Init() tea.Cmd {
	return nil
}

func (m PinEntryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.shaking {
			return m, nil // ignore input during shake
		}
		s := msg.String()
		if s == "ctrl+c" {
			return m, tea.Quit
		}
		if len(s) == 1 && s[0] >= '0' && s[0] <= '9' {
			m.digits = append(m.digits, rune(s[0]))
			if len(m.digits) == 4 {
				pin := string(m.digits)
				if VerifyPin(pin) {
					return m, func() tea.Msg { return switchScreenMsg{screen: ScreenHome} }
				}
				// Wrong PIN — shake
				m.shaking = true
				m.shakeTick = 0
				return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return pinShakeTickMsg{} })
			}
		}
	case pinShakeTickMsg:
		m.shakeTick++
		if m.shakeTick >= 4 {
			m.shaking = false
			m.digits = nil
			return m, nil
		}
		return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return pinShakeTickMsg{} })
	}
	return m, nil
}

func (m PinEntryModel) View() string {
	envSt := lipgloss.NewStyle().Foreground(colorAccent)

	var b strings.Builder
	b.WriteString(envSt.Render("✉") + "\n")
	b.WriteString("\n")
	b.WriteString(renderDots(len(m.digits), 4, m.shaking) + "\n")

	inner := lipgloss.NewStyle().
		Width(contentWidth()).
		Height(10).
		AlignVertical(lipgloss.Center).
		AlignHorizontal(lipgloss.Center).
		Render(b.String())
	return screenBox().Render(inner)
}

// --- PIN Setup (optional, after registration) ---

type PinSetupModel struct {
	app       *AppState
	digits    []rune
	first     string // saved first entry
	phase     int    // 0 = verify old (if exists), 1 = enter new, 2 = confirm new
	err       string
	hasPin    bool // whether a PIN existed when we started
	shaking   bool
	shakeTick int
	origin    Screen // screen to return to when done
}

func NewPinSetupModel(app *AppState) PinSetupModel {
	m := PinSetupModel{app: app, hasPin: PinFileExists(), origin: ScreenSettings}
	if m.hasPin {
		m.phase = 0 // verify old PIN first
	} else {
		m.phase = 1 // go straight to choose new PIN
	}
	return m
}

func NewPinSetupModelWithOrigin(app *AppState, origin Screen) PinSetupModel {
	m := NewPinSetupModel(app)
	m.origin = origin
	return m
}

func (m PinSetupModel) Init() tea.Cmd {
	return nil
}

func (m PinSetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.shaking {
			return m, nil
		}
		s := msg.String()
		switch {
		case s == "ctrl+c":
			return m, tea.Quit
		case (s == "b" || s == "esc") && len(m.digits) == 0:
			if m.phase <= 1 {
				// Back to settings (or home if we came from registration)
				return m, func() tea.Msg { return switchScreenMsg{screen: m.origin} }
			}
			// Phase 2 (confirm): start over at choose new
			m.phase = 1
			m.first = ""
			m.digits = nil
			m.err = ""
		case s == "enter" && len(m.digits) == 0 && m.phase == 1 && !m.hasPin:
			// Skip PIN setup (only for new users without a PIN)
			return m, func() tea.Msg { return switchScreenMsg{screen: m.origin} }
		case len(s) == 1 && s[0] >= '0' && s[0] <= '9':
			m.digits = append(m.digits, rune(s[0]))
			m.err = ""
			if len(m.digits) == 4 {
				pin := string(m.digits)
				switch m.phase {
				case 0: // verify old PIN
					if VerifyPin(pin) {
						m.digits = nil
						m.phase = 1
						m.err = ""
					} else {
						m.shaking = true
						m.shakeTick = 0
						return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return pinShakeTickMsg{} })
					}
				case 1: // choose new
					m.first = pin
					m.digits = nil
					m.phase = 2
				case 2: // confirm new
					if pin == m.first {
						if err := SavePinHash(pin); err != nil {
							m.err = fmt.Sprintf("saving pin: %v", err)
							m.digits = nil
							return m, nil
						}
						return m, func() tea.Msg { return switchScreenMsg{screen: m.origin} }
					}
					m.err = "pins don't match"
					m.phase = 1
					m.first = ""
					m.digits = nil
				}
			}
		}
	case pinShakeTickMsg:
		m.shakeTick++
		if m.shakeTick >= 4 {
			m.shaking = false
			m.digits = nil
			return m, nil
		}
		return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return pinShakeTickMsg{} })
	}
	return m, nil
}

func (m PinSetupModel) View() string {
	envSt := lipgloss.NewStyle().Foreground(colorAccent)

	var b strings.Builder
	b.WriteString(envSt.Render("✉") + "\n")
	b.WriteString("\n")
	b.WriteString(renderDots(len(m.digits), 4, m.shaking) + "\n")
	b.WriteString("\n")

	if m.err != "" {
		b.WriteString(errorStyle.Render(m.err) + "\n")
		b.WriteString("\n")
	}

	switch m.phase {
	case 0:
		b.WriteString(mutedStyle.Render("enter current pin") + "\n")
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[b] back"))
	case 1:
		if m.hasPin {
			b.WriteString(mutedStyle.Render("choose a new pin") + "\n")
		} else {
			b.WriteString(mutedStyle.Render("choose a 4-digit pin") + "\n")
		}
		b.WriteString("\n")
		if m.hasPin {
			b.WriteString(helpStyle.Render("[b] back"))
		} else {
			b.WriteString(helpStyle.Render("[enter] skip  [b] back"))
		}
	case 2:
		b.WriteString(mutedStyle.Render("confirm your pin") + "\n")
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("[b] start over"))
	}

	inner := lipgloss.NewStyle().
		Width(contentWidth()).
		Height(10).
		AlignVertical(lipgloss.Center).
		AlignHorizontal(lipgloss.Center).
		Render(b.String())
	return screenBox().Render(inner)
}
