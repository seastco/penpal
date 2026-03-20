package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	pencrypto "github.com/seastco/penpal/internal/crypto"
	"github.com/seastco/penpal/internal/models"
)

// RegisterModel handles the registration and recovery flows.
type RegisterModel struct {
	app      *AppState
	step     int // 0=choice, 1=username, 2=seed display, 3=city selection, 4=recover
	input    textinput.Model
	err      string
	mnemonic string
	disc     string // assigned discriminator

	// City selection
	cityInput   textinput.Model
	cityResults []cityMatch
	cityIdx     int

	// Choice / recovery
	choiceIdx    int // 0=register, 1=recover
	recoverInput textinput.Model
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
	"PT": "Portugal",
}

type registeredMsg struct {
	disc string
}

type recoveredMsg struct {
	user models.User
}

type citiesSearchedMsg struct {
	results []cityMatch
}

func NewRegisterModel(app *AppState) RegisterModel {
	ti := textinput.New()
	ti.Placeholder = "username"
	ti.CharLimit = 32
	ti.Width = contentWidth() - 8

	ci := textinput.New()
	ci.Placeholder = "city name"
	ci.CharLimit = 50
	ci.Width = contentWidth() - 8

	ri := textinput.New()
	ri.Placeholder = "word1 word2 word3 ... word12"
	ri.CharLimit = 200
	ri.Width = contentWidth() - 8

	return RegisterModel{
		app:          app,
		step:         0,
		input:        ti,
		cityInput:    ci,
		recoverInput: ri,
	}
}

func (m RegisterModel) Init() tea.Cmd {
	return nil
}

func (m RegisterModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.step {
	case 0:
		return m.updateChoice(msg)
	case 1:
		return m.updateUsername(msg)
	case 2:
		return m.updateSeed(msg)
	case 3:
		return m.updateCity(msg)
	case 4:
		return m.updateRecover(msg)
	}
	return m, nil
}

func (m RegisterModel) updateChoice(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.choiceIdx = 0
		case "down", "j":
			m.choiceIdx = 1
		case "enter":
			if m.choiceIdx == 0 {
				m.step = 1
				m.input.Focus()
				return m, textinput.Blink
			}
			m.step = 4
			m.recoverInput.Focus()
			return m, textinput.Blink
		case "ctrl+c":
			return m, tea.Quit
		}
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

			m.step = 2
			m.err = ""
			return m, nil
		case "esc":
			m.step = 0
			m.err = ""
			m.input.SetValue("")
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
			m.step = 3
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
					// Clear stale pin from any previous account
					clearPin()
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
					m.app.Network.SetAuthCredentials(m.app.Username, resp.Discriminator, m.app.PrivateKey)

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

func (m RegisterModel) updateRecover(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			mnemonic := strings.TrimSpace(m.recoverInput.Value())
			words := strings.Fields(mnemonic)
			if len(words) != 12 {
				m.err = "recovery phrase must be exactly 12 words"
				return m, nil
			}
			mnemonic = strings.Join(words, " ")

			pub, priv, err := pencrypto.KeypairFromMnemonic(mnemonic)
			if err != nil {
				m.err = "invalid recovery phrase"
				return m, nil
			}

			m.app.PublicKey = pub
			m.app.PrivateKey = priv
			m.mnemonic = mnemonic
			m.err = ""

			return m, func() tea.Msg {
				ctx := context.Background()
				resp, err := m.app.Network.Recover(ctx, pub)
				if err != nil {
					return errMsg{err: err}
				}
				return recoveredMsg{user: resp.User}
			}
		case "esc":
			m.step = 0
			m.err = ""
			m.recoverInput.SetValue("")
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		default:
			var cmd tea.Cmd
			m.recoverInput, cmd = m.recoverInput.Update(msg)
			return m, cmd
		}
	case recoveredMsg:
		clearPin()
		if err := pencrypto.SaveKeyFile(m.app.PublicKey, m.app.PrivateKey); err != nil {
			m.err = fmt.Sprintf("saving key: %v", err)
			return m, nil
		}
		m.app.UserID = msg.user.ID
		m.app.Username = msg.user.Username
		m.app.Discriminator = msg.user.Discriminator
		m.app.HomeCity = msg.user.HomeCity
		m.app.Network.SetAuthCredentials(msg.user.Username, msg.user.Discriminator, m.app.PrivateKey)
		saveIdentity(msg.user.Username, msg.user.Discriminator)

		return m, func() tea.Msg {
			return switchScreenMsg{screen: ScreenHome}
		}
	case errMsg:
		m.err = msg.err.Error()
	}
	return m, nil
}

func (m RegisterModel) View() string {
	switch m.step {
	case 0:
		return m.viewChoice()
	case 1:
		return m.viewUsername()
	case 2:
		return m.viewSeed()
	case 3:
		return m.viewCity()
	case 4:
		return m.viewRecover()
	}
	return ""
}

func (m RegisterModel) viewChoice() string {
	title := titleStyle.Render("PENPAL")

	body := "\nWelcome! What would you like to do?\n\n"
	options := []string{"Create new account", "Recover existing account"}
	for i, opt := range options {
		if i == m.choiceIdx {
			body += selectedStyle.Render("> "+opt) + "\n"
		} else {
			body += mutedStyle.Render("  "+opt) + "\n"
		}
	}
	body += "\n" + helpStyle.Render("[enter] select  [up/down] navigate")

	content := title + "\n" + body
	return screenBox().Render(content)
}

func (m RegisterModel) viewUsername() string {
	title := titleStyle.Render("PENPAL")
	body := "\nPick a username to join the network.\n"
	body += fmt.Sprintf("\nusername: %s\n", m.input.View())
	if m.err != "" {
		body += "\n" + errorStyle.Render(m.err) + "\n"
	}
	body += "\n" + helpStyle.Render("[enter] claim  [esc] back")
	content := title + "\n" + body
	return screenBox().Render(content)
}

func (m RegisterModel) viewSeed() string {
	title := titleStyle.Render("RECOVERY PHRASE")

	body := "\nWrite down these 12 words and store them somewhere safe.\n"
	body += "This is the ONLY way to recover your account.\n\n"

	words := strings.Fields(m.mnemonic)
	for i, w := range words {
		if i < 6 {
			body += fmt.Sprintf("  %2d. %-12s", i+1, w)
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

func (m RegisterModel) viewRecover() string {
	title := titleStyle.Render("ACCOUNT RECOVERY")

	body := "\nEnter your 12-word recovery phrase:\n\n"
	body += fmt.Sprintf("%s\n", m.recoverInput.View())

	if m.err != "" {
		body += "\n" + errorStyle.Render(m.err) + "\n"
	}
	body += "\n" + helpStyle.Render("[enter] recover  [esc] back")

	content := title + "\n" + divider(contentWidth()) + "\n" + body
	return screenBox().Render(content)
}
