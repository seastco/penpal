package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	pencrypto "github.com/stove/penpal/internal/crypto"
	"github.com/stove/penpal/internal/protocol"
)

type settingsMode int

const (
	settingsMenu settingsMode = iota
	settingsCity
	settingsTheme
)

type homeCityUpdatedMsg struct{}

type SettingsModel struct {
	app    *AppState
	cursor int
	mode   settingsMode

	// City picker state
	cityInput   textinput.Model
	cityResults []cityMatch
	cityIdx     int

	// Theme picker state
	themeIdx    int
	prevTheme   string // to revert on cancel

}

func NewSettingsModel(app *AppState) SettingsModel {
	ti := textinput.New()
	ti.Placeholder = "city name..."
	ti.CharLimit = 40

	// Find current theme index
	themeIdx := 0
	for i, t := range themes {
		if strings.EqualFold(t.Name, app.ThemeName) {
			themeIdx = i
			break
		}
	}

	return SettingsModel{
		app:      app,
		cityInput: ti,
		themeIdx:  themeIdx,
	}
}

func (m SettingsModel) Init() tea.Cmd { return nil }

func (m SettingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case settingsCity:
		return m.updateCity(msg)
	case settingsTheme:
		return m.updateTheme(msg)
	default:
		return m.updateMenu(msg)
	}
}

func (m SettingsModel) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < 2 {
				m.cursor++
			}
		case "enter":
			switch m.cursor {
			case 0: // Home City
				m.mode = settingsCity
				m.cityInput.SetValue("")
				m.cityResults = nil
				m.cityIdx = 0
				m.cityInput.Focus()
				return m, textinput.Blink
			case 1: // PIN
				return m, func() tea.Msg { return switchScreenMsg{screen: ScreenPinSetup} }
			case 2: // Theme
				m.mode = settingsTheme
				m.prevTheme = m.app.ThemeName
				return m, nil
			}
		case "b", "esc":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenHome} }
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m SettingsModel) updateCity(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if len(m.cityResults) > 0 && m.cityIdx < len(m.cityResults) {
				city := m.cityResults[m.cityIdx]
				suffix := city.state
				if city.country != "" && city.country != "US" {
					suffix = city.country
				}
				homeCity := fmt.Sprintf("%s, %s", city.name, suffix)
				return m, func() tea.Msg {
					ctx := context.Background()
					_, err := m.app.Network.Send(ctx, protocol.MsgUpdateHomeCity, protocol.UpdateHomeCityRequest{
						City: homeCity,
						Lat:  city.lat,
						Lng:  city.lng,
					})
					if err != nil {
						return errMsg{err: err}
					}
					return homeCityUpdatedMsg{}
				}
			}
		case "up":
			if m.cityIdx > 0 {
				m.cityIdx--
			}
		case "down":
			if m.cityIdx < len(m.cityResults)-1 {
				m.cityIdx++
			}
		case "ctrl+b", "esc":
			m.mode = settingsMenu
			m.cityInput.Blur()
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		default:
			var cmd tea.Cmd
			m.cityInput, cmd = m.cityInput.Update(msg)
			query := m.cityInput.Value()
			if len(query) >= 2 {
				return m, tea.Batch(cmd, func() tea.Msg {
					ctx := context.Background()
					cities, err := m.app.Network.SearchCities(ctx, query)
					if err != nil {
						return nil
					}
					var results []cityMatch
					for _, c := range cities {
						results = append(results, cityMatch{
							name: c.Name, state: c.State, country: c.Country,
							lat: c.Lat, lng: c.Lng,
						})
					}
					return citiesSearchedMsg{results: results}
				})
			}
			m.cityResults = nil
			return m, cmd
		}
	case citiesSearchedMsg:
		m.cityResults = msg.results
		m.cityIdx = 0
	case homeCityUpdatedMsg:
		// Update local state with the selected city
		if len(m.cityResults) > 0 && m.cityIdx < len(m.cityResults) {
			city := m.cityResults[m.cityIdx]
			suffix := city.state
			if city.country != "" && city.country != "US" {
				suffix = city.country
			}
			m.app.HomeCity = fmt.Sprintf("%s, %s", city.name, suffix)
		}
		m.mode = settingsMenu
		m.cityInput.Blur()
		m.cityResults = nil
	}
	return m, nil
}

func (m SettingsModel) updateTheme(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.themeIdx > 0 {
				m.themeIdx--
				applyTheme(themes[m.themeIdx])
			}
		case "down", "j":
			if m.themeIdx < len(themes)-1 {
				m.themeIdx++
				applyTheme(themes[m.themeIdx])
			}
		case "enter":
			m.app.ThemeName = themes[m.themeIdx].Name
			if dir, err := pencrypto.PenpalDir(); err == nil {
				saveTheme(dir, m.app.ThemeName)
			}
			m.mode = settingsMenu
			return m, nil
		case "esc", "b":
			// Revert to previous theme
			applyTheme(themeByName(m.prevTheme))
			m.themeIdx = 0
			for i, t := range themes {
				if strings.EqualFold(t.Name, m.prevTheme) {
					m.themeIdx = i
					break
				}
			}
			m.mode = settingsMenu
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m SettingsModel) View() string {
	switch m.mode {
	case settingsCity:
		return m.viewCity()
	case settingsTheme:
		return m.viewTheme()
	default:
		return m.viewMenu()
	}
}

func (m SettingsModel) viewMenu() string {
	title := titleStyle.Render("PENPAL — SETTINGS")
	div := divider(contentWidth())

	items := []struct {
		label string
		value string
	}{
		{"Home City", m.app.HomeCity},
		{"PIN", m.pinDisplay()},
		{"Theme", m.app.ThemeName},
	}

	var rows []string
	for i, item := range items {
		cursor := "  "
		style := mutedStyle
		if i == m.cursor {
			cursor = selectedStyle.Render("> ")
			style = menuStyle
		}
		label := style.Render(fmt.Sprintf("%-12s", item.label))
		value := mutedStyle.Render(item.value)
		rows = append(rows, cursor+label+value)
	}

	help := "  " + helpStyle.Render("[enter] edit  [b] back")

	content := fmt.Sprintf("%s\n%s\n\n%s\n\n%s",
		title, div,
		strings.Join(rows, "\n"),
		help,
	)

	return screenBox().Render(content)
}

func (m SettingsModel) pinDisplay() string {
	if PinFileExists() {
		return "••••"
	}
	return "not set"
}

func (m SettingsModel) viewCity() string {
	title := titleStyle.Render("PENPAL — HOME CITY")
	div := divider(contentWidth())

	prompt := menuStyle.Render("Where are you based?")
	input := fmt.Sprintf("city: %s", m.cityInput.View())

	var cityList string
	for i, c := range m.cityResults {
		cursor := "  "
		if i == m.cityIdx {
			suffix := c.state
			if c.country != "" && c.country != "US" {
				if name, ok := countryNames[c.country]; ok {
					suffix = name
				} else {
					suffix = c.country
				}
			}
			cursor = selectedStyle.Render("> ") + selectedStyle.Render(fmt.Sprintf("%s, %s", c.name, suffix))
		} else {
			suffix := c.state
			if c.country != "" && c.country != "US" {
				if name, ok := countryNames[c.country]; ok {
					suffix = name
				} else {
					suffix = c.country
				}
			}
			cursor += mutedStyle.Render(fmt.Sprintf("%s, %s", c.name, suffix))
		}
		cityList += cursor + "\n"
	}

	help := helpStyle.Render("[enter] select  [ctrl+b] back")

	content := fmt.Sprintf("%s\n%s\n\n%s\n\n%s\n%s\n\n%s",
		title, div,
		prompt,
		input,
		cityList,
		help,
	)

	return screenBox().Render(content)
}

func (m SettingsModel) viewTheme() string {
	title := titleStyle.Render("PENPAL — THEME")
	div := divider(contentWidth())

	var rows []string
	for i, t := range themes {
		if i == m.themeIdx {
			rows = append(rows, selectedStyle.Render("> ")+selectedStyle.Render(t.Name))
		} else {
			rows = append(rows, "  "+mutedStyle.Render(t.Name))
		}
	}

	help := "  " + helpStyle.Render("[enter] apply  [b] back")

	content := fmt.Sprintf("%s\n%s\n\n%s\n\n%s",
		title, div,
		strings.Join(rows, "\n"),
		help,
	)

	return screenBox().Render(content)
}
