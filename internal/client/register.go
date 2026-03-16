package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	pencrypto "github.com/stove/penpal/internal/crypto"
)

// RegisterModel handles the registration flow.
type RegisterModel struct {
	app      *AppState
	step     int // 0=username, 1=seed display, 2=city selection
	input    textinput.Model
	err      string
	mnemonic string
	disc     string // assigned discriminator

	// City selection
	cityInput   textinput.Model
	cityResults []cityMatch
	cityIdx     int
}

type cityMatch struct {
	name    string
	state   string
	country string // ISO 3166-1 alpha-2; "US" for domestic
	lat     float64
	lng     float64
}

// countryNames maps country codes to display names for international cities.
var countryNames = map[string]string{
	"ES": "Spain",
}

type registeredMsg struct {
	disc string
}

type citiesSearchedMsg struct {
	results []cityMatch
}

func NewRegisterModel(app *AppState) RegisterModel {
	ti := textinput.New()
	ti.Placeholder = "username"
	ti.Focus()
	ti.CharLimit = 32
	ti.Width = contentWidth() - 8

	ci := textinput.New()
	ci.Placeholder = "city name"
	ci.CharLimit = 50
	ci.Width = contentWidth() - 8

	return RegisterModel{
		app:       app,
		step:      0,
		input:     ti,
		cityInput: ci,
	}
}

func (m RegisterModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m RegisterModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.step {
	case 0:
		return m.updateUsername(msg)
	case 1:
		return m.updateSeed(msg)
	case 2:
		return m.updateCity(msg)
	}
	return m, nil
}

func (m RegisterModel) updateUsername(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			username := strings.ToLower(strings.TrimSpace(m.input.Value()))
			if len(username) < 1 {
				m.err = "username cannot be empty"
				return m, nil
			}
			if len(username) > 32 {
				m.err = "username must be 32 characters or less"
				return m, nil
			}
			// Generate keypair
			mnemonic, err := pencrypto.GenerateMnemonic()
			if err != nil {
				m.err = fmt.Sprintf("key generation failed: %v", err)
				return m, nil
			}
			m.mnemonic = mnemonic
			m.app.Username = username

			pub, priv, err := pencrypto.KeypairFromMnemonic(mnemonic)
			if err != nil {
				m.err = fmt.Sprintf("key derivation failed: %v", err)
				return m, nil
			}
			m.app.PublicKey = pub
			m.app.PrivateKey = priv

			// We'll register with the server after city selection (step 2)
			// For now, move to seed display
			m.step = 1
			m.err = ""
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m RegisterModel) updateSeed(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			m.step = 2
			m.cityInput.Focus()
			return m, textinput.Blink
		case "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m RegisterModel) updateCity(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if len(m.cityResults) > 0 && m.cityIdx < len(m.cityResults) {
				city := m.cityResults[m.cityIdx]
				// Register with server
				return m, func() tea.Msg {
					ctx := context.Background()
					// Use country code as suffix for international cities
					suffix := city.state
					if city.country != "" && city.country != "US" {
						suffix = city.country
					}
					homeCity := fmt.Sprintf("%s, %s", city.name, suffix)
					resp, err := m.app.Network.Register(ctx,
						m.app.Username, m.app.PublicKey,
						homeCity,
						city.lat, city.lng,
					)
					if err != nil {
						return errMsg{err: err}
					}
					// Save key locally
					if err := pencrypto.SaveKeyFile(m.app.PublicKey, m.app.PrivateKey); err != nil {
						return errMsg{err: fmt.Errorf("saving key: %w", err)}
					}
					// Save identity info
					m.app.UserID = resp.UserID
					m.app.Discriminator = resp.Discriminator
					m.app.HomeCity = homeCity

					// Save username/discriminator alongside key
					saveIdentity(m.app.Username, m.app.Discriminator)

					return registeredMsg{disc: resp.Discriminator}
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
		case "esc":
			m.cityInput.SetValue("")
			m.cityResults = nil
			m.cityIdx = 0
		case "ctrl+c":
			return m, tea.Quit
		default:
			var cmd tea.Cmd
			m.cityInput, cmd = m.cityInput.Update(msg)
			// Search cities on input change
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
			return m, cmd
		}
	case citiesSearchedMsg:
		m.cityResults = msg.results
		m.cityIdx = 0
	case registeredMsg:
		m.disc = msg.disc
		return m, func() tea.Msg {
			return switchScreenMsg{screen: ScreenPinSetup}
		}
	case errMsg:
		m.err = msg.err.Error()
	}
	return m, nil
}

func (m RegisterModel) View() string {
	switch m.step {
	case 0:
		return m.viewUsername()
	case 1:
		return m.viewSeed()
	case 2:
		return m.viewCity()
	}
	return ""
}

func (m RegisterModel) viewUsername() string {
	title := titleStyle.Render("PENPAL")
	body := "\n  No account found. Pick a username to join the network.\n"
	body += fmt.Sprintf("\n  username: %s\n", m.input.View())
	if m.err != "" {
		body += "\n" + errorStyle.Render(m.err) + "\n"
	}
	body += "\n" + helpStyle.Render("[enter] claim")
	content := title + "\n" + body
	return screenBox().Render(content)
}

func (m RegisterModel) viewSeed() string {
	title := titleStyle.Render("RECOVERY PHRASE")

	body := "\n  Write down these 12 words and store them somewhere safe.\n"
	body += "  This is the ONLY way to recover your account.\n\n"

	words := strings.Fields(m.mnemonic)
	for i, w := range words {
		if i < 6 {
			body += fmt.Sprintf("   %2d. %-12s", i+1, w)
			if i+6 < len(words) {
				body += fmt.Sprintf("%2d. %s", i+7, words[i+6])
			}
			body += "\n"
		}
	}
	body += "\n" + helpStyle.Render("[enter] I've written these down")

	content := title + "\n" + divider(contentWidth()) + "\n" + body
	return screenBox().Render(content)
}

func (m RegisterModel) viewCity() string {
	title := titleStyle.Render("PENPAL — HOME CITY")

	body := "\nWhere are you based?\n"
	body += fmt.Sprintf("\ncity: %s\n", m.cityInput.View())

	for i, c := range m.cityResults {
		prefix := "  "
		if i == m.cityIdx {
			prefix = "> "
		}
		var name string
		if c.country != "" && c.country != "US" {
			cname := c.country
			if full, ok := countryNames[c.country]; ok {
				cname = full
			}
			name = fmt.Sprintf("%s (%s)", c.name, cname)
		} else {
			name = fmt.Sprintf("%s, %s", c.name, c.state)
		}
		if i == m.cityIdx {
			body += selectedStyle.Render(prefix+name) + "\n"
		} else {
			body += mutedStyle.Render(prefix+name) + "\n"
		}
	}

	if m.err != "" {
		body += "\n" + errorStyle.Render(m.err) + "\n"
	}
	body += "\n" + helpStyle.Render("[enter] select  [esc] clear")

	content := title + "\n" + divider(contentWidth()) + "\n" + body
	return screenBox().Render(content)
}
