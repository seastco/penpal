package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stove/penpal/internal/protocol"
)

// AddressBookModel shows the user's contacts.
type AddressBookModel struct {
	app      *AppState
	contacts []protocol.ContactItem
	cursor        int
	viewport      viewport.Model
	loading       bool
	err           string
	confirmDelete bool
}

func NewAddressBookModel(app *AppState) AddressBookModel {
	vp := viewport.New(contentWidth(), viewportHeight()-4)
	vp.KeyMap = viewport.KeyMap{}
	m := AddressBookModel{app: app, loading: true, viewport: vp}
	return m.syncViewport()
}

func (m AddressBookModel) Init() tea.Cmd {
	return func() tea.Msg {
		contacts, err := m.app.Network.GetContacts(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		return contactsLoadedMsg{contacts: contacts}
	}
}

func (m AddressBookModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.confirmDelete {
			if msg.String() == "y" {
				m.confirmDelete = false
				if m.cursor < len(m.contacts) {
					c := m.contacts[m.cursor]
					return m, func() tea.Msg {
						ctx := context.Background()
						_, err := m.app.Network.Send(ctx, protocol.MsgDeleteContact, protocol.DeleteContactRequest{
							ContactID: c.UserID,
						})
						if err != nil {
							return errMsg{err: err}
						}
						contacts, err := m.app.Network.GetContacts(ctx)
						if err != nil {
							return errMsg{err: err}
						}
						return contactsLoadedMsg{contacts: contacts}
					}
				}
			} else {
				m.confirmDelete = false
			}
			m = m.syncViewport()
			return m, nil
		}
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.contacts)-1 {
				m.cursor++
			}
		case "a":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenAddContact} }
		case "d":
			if len(m.contacts) > 0 {
				m.confirmDelete = true
			}
		case "b", "esc":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenHome} }
		}
	case tea.WindowSizeMsg:
		m.viewport.Width = contentWidth()
	case contactsLoadedMsg:
		m.contacts = msg.contacts
		m.loading = false
		if m.cursor >= len(m.contacts) && m.cursor > 0 {
			m.cursor = len(m.contacts) - 1
		}
	case errMsg:
		m.err = msg.err.Error()
		m.loading = false
	}
	m = m.syncViewport()
	return m, nil
}

func (m AddressBookModel) syncViewport() AddressBookModel {
	bh := adaptiveBoxHeight(len(m.contacts), 10)
	m.viewport.Height = bh - 10

	var content string
	if m.err != "" {
		content = errorStyle.Render(m.err)
	} else if len(m.contacts) == 0 {
		content = mutedStyle.Render("no contacts yet")
	} else {
		var b strings.Builder
		for i, c := range m.contacts {
			prefix := "  "
			if i == m.cursor {
				prefix = "> "
			}
			line := fmt.Sprintf("%-14s %s", c.Username, mutedStyle.Render(c.HomeCity))
			if i == m.cursor {
				b.WriteString(selectedStyle.Render(prefix+line) + "\n")
			} else {
				b.WriteString(prefix + line + "\n")
			}
		}
		content = b.String()
	}

	yOffset := m.viewport.YOffset
	m.viewport.SetContent(content)
	if len(m.contacts) > 0 {
		m.viewport.SetYOffset(yOffset)
		if m.cursor < m.viewport.YOffset {
			m.viewport.SetYOffset(m.cursor)
		} else if m.cursor >= m.viewport.YOffset+m.viewport.Height {
			m.viewport.SetYOffset(m.cursor - m.viewport.Height + 1)
		}
	}
	return m
}

func (m AddressBookModel) View() string {
	title := titleStyle.Render("ADDRESS BOOK")
	header := title + "\n" + divider(contentWidth()) + "\n"
	header += fmt.Sprintf("\nyour address: %s\n\ncontacts:\n", selectedStyle.Render(m.app.Address()))
	if len(m.contacts) == 0 {
		body := mutedStyle.Render("no contacts yet")
		if m.err != "" {
			body = errorStyle.Render(m.err)
		}
		return emptyScreenView(header, body, "[a] add new  [b] back")
	}
	m = m.syncViewport()
	bh := adaptiveBoxHeight(len(m.contacts), 10)
	var footer string
	if m.confirmDelete && m.cursor < len(m.contacts) {
		name := m.contacts[m.cursor].Username
		footer = "\n\n" + errorStyle.Render(fmt.Sprintf("delete %s? [y] yes  [n] no", name))
	} else {
		footer = "\n\n" + helpStyle.Render("[a] add new  [d] delete  [b] back")
	}
	return screenBox().Height(bh).Render(header + m.viewport.View() + footer)
}

// AddContactModel handles adding a new contact.
type AddContactModel struct {
	app   *AppState
	input textinput.Model
	err   string
	added *protocol.ContactItem
}

func NewAddContactModel(app *AppState) AddContactModel {
	ti := textinput.New()
	ti.Placeholder = "username#0000"
	ti.Focus()
	ti.CharLimit = 37
	ti.Width = contentWidth() - 8

	return AddContactModel{
		app:   app,
		input: ti,
	}
}

type contactAddedMsg struct {
	contact *protocol.ContactItem
}

func (m AddContactModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m AddContactModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// After successful add, handle done/add-another
		if m.added != nil {
			switch msg.String() {
			case "enter":
				return m, func() tea.Msg { return switchScreenMsg{screen: ScreenAddressBook} }
			case "n":
				return NewAddContactModel(m.app), textinput.Blink
			}
			return m, nil
		}
		switch msg.String() {
		case "enter":
			addr := strings.TrimSpace(m.input.Value())
			parts := strings.SplitN(addr, "#", 2)
			if len(parts) != 2 || len(parts[1]) != 4 {
				m.err = "invalid address format (use username#0000)"
				return m, nil
			}
			username, disc := parts[0], parts[1]
			return m, func() tea.Msg {
				contact, err := m.app.Network.AddContact(context.Background(), username, disc)
				if err != nil {
					return errMsg{err: err}
				}
				return contactAddedMsg{contact: contact}
			}
		case "ctrl+b":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenAddressBook} }
		}
	case contactAddedMsg:
		m.added = msg.contact
	case errMsg:
		m.err = msg.err.Error()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m AddContactModel) View() string {
	title := titleStyle.Render("ADD CONTACT")
	content := title + "\n" + divider(contentWidth()) + "\n"

	if m.added != nil {
		content += fmt.Sprintf("\nadded %s (%s)\n",
			selectedStyle.Render(m.added.Username),
			mutedStyle.Render(m.added.HomeCity))
		content += "\n" + helpStyle.Render("[enter] done  [n] add another")
	} else {
		content += fmt.Sprintf("\naddress: %s\n", m.input.View())
		if m.err != "" {
			content += "\n" + errorStyle.Render(m.err) + "\n"
		}
		content += "\n" + helpStyle.Render("[enter] add  [ctrl+b] back")
	}
	return screenBox().Render(content)
}
