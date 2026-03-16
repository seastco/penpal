package client

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Screen represents which screen is currently displayed.
type Screen int

const (
	ScreenHome Screen = iota
	ScreenRegister
	ScreenRegisterSeed
	ScreenRegisterCity
	ScreenInbox
	ScreenReadLetter
	ScreenInTransit
	ScreenSent
	ScreenTracking
	ScreenCompose
	ScreenComposeBody
	ScreenComposeStamps
	ScreenComposeShipping
	ScreenAddressBook
	ScreenAddContact
	ScreenStamps
	ScreenSplash
	ScreenPinEntry
	ScreenPinSetup
	ScreenSettings
)

// HomeModel is the main menu screen.
type HomeModel struct {
	app       *AppState
	inboxNew  int
	transitN  int
}

func NewHomeModel(app *AppState) HomeModel {
	return HomeModel{app: app}
}

func (m HomeModel) Init() tea.Cmd {
	return nil
}

func (m HomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "i":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenInbox} }
		case "s":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenSent} }
		case "c":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenCompose} }
		case "m":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenStamps} }
		case "a":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenAddressBook} }
		case "p":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenSettings} }
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case dataRefreshMsg:
		m.inboxNew = msg.inboxNew
		m.transitN = msg.transitCount
	}
	return m, nil
}

func (m HomeModel) View() string {
	title := headerLine(titleStyle.Render("PENPAL"), mutedStyle.Render(m.app.Address()))

	inbox := fmt.Sprintf("%s Inbox", menuKeyStyle.Render("[i]"))
	if m.inboxNew > 0 {
		inbox += "  " + newStyle.Render(fmt.Sprintf("(%d new)", m.inboxNew))
	}

	sent := fmt.Sprintf("%s Sent", menuKeyStyle.Render("[s]"))
	compose := fmt.Sprintf("%s Compose", menuKeyStyle.Render("[c]"))
	stamps := fmt.Sprintf("%s Stamps", menuKeyStyle.Render("[m]"))
	addressBook := fmt.Sprintf("%s Address Book", menuKeyStyle.Render("[a]"))
	settings := fmt.Sprintf("%s Settings", menuKeyStyle.Render("[p]"))
	quit := fmt.Sprintf("%s Quit", menuKeyStyle.Render("[q]"))

	content := fmt.Sprintf("%s\n%s\n\n%s\n%s\n%s\n%s\n%s\n%s\n\n%s",
		title,
		divider(contentWidth()),
		inbox,
		sent,
		compose,
		stamps,
		addressBook,
		settings,
		quit,
	)

	return screenBox().Render(content)
}

// --- Messages ---

type switchScreenMsg struct {
	screen Screen
}

type dataRefreshMsg struct {
	inboxNew     int
	transitCount int
}

type errMsg struct {
	err error
}

type backToInboxMsg struct{}
