package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	pencrypto "github.com/seastco/penpal/internal/crypto"
	"github.com/seastco/penpal/internal/models"
	"github.com/seastco/penpal/internal/protocol"
)

// InboxModel shows delivered letters.
type InboxModel struct {
	app           *AppState
	items         []protocol.InboxItem
	cursor        int
	viewport      viewport.Model
	loading       bool
	loadingMore   bool
	hasMore       bool
	confirmDelete bool
	err           string
}

func NewInboxModel(app *AppState) InboxModel {
	vp := viewport.New(viewport.WithWidth(contentWidth()), viewport.WithHeight(viewportHeight()))
	vp.KeyMap = viewport.KeyMap{}
	m := InboxModel{app: app, loading: true, viewport: vp}
	return m.syncViewport()
}

type inboxLoadedMsg struct {
	items   []protocol.InboxItem
	hasMore bool
	append  bool // true = subsequent page (append to existing), false = first page (replace)
}

func (m InboxModel) Init() tea.Cmd {
	return m.fetchInbox(nil, false)
}

func (m InboxModel) fetchInbox(before *time.Time, append bool) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.app.Network.GetInbox(context.Background(), before)
		if err != nil {
			return errMsg{err: err}
		}
		return inboxLoadedMsg{items: resp.Letters, hasMore: resp.HasMore, append: append}
	}
}

func (m InboxModel) maybePrefetch() tea.Cmd {
	if m.hasMore && !m.loadingMore && m.cursor >= len(m.items)-50 && len(m.items) > 0 {
		last := m.items[len(m.items)-1]
		cursor := last.DeliveredAt
		m.loadingMore = true
		return m.fetchInbox(&cursor, true)
	}
	return nil
}

func (m InboxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.confirmDelete {
			if msg.String() == "y" && m.cursor < len(m.items) {
				item := m.items[m.cursor]
				m.confirmDelete = false
				return m, func() tea.Msg {
					ctx := context.Background()
					_, err := m.app.Network.Send(ctx, protocol.MsgDeleteLetter, protocol.DeleteLetterRequest{
						MessageID: item.MessageID,
					})
					if err != nil {
						return errMsg{err: err}
					}
					// Refetch inbox
					resp, err := m.app.Network.GetInbox(ctx, nil)
					if err != nil {
						return errMsg{err: err}
					}
					return inboxLoadedMsg{items: resp.Letters, hasMore: resp.HasMore}
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
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
			m = m.syncViewport()
			return m, m.maybePrefetch()
		case "d":
			if len(m.items) > 0 {
				m.confirmDelete = true
			}
		case "enter":
			if len(m.items) > 0 {
				item := m.items[m.cursor]
				body, cached := m.app.DecryptedBodies[item.MessageID]
				if !cached {
					if err := pencrypto.VerifyAndPinKey(item.SenderID.String(), item.SenderPubKey); err != nil {
						body = fmt.Sprintf("(key verification failed: %v)", err)
					} else if plaintext, err := pencrypto.Decrypt(item.EncryptedBody, m.app.PrivateKey, item.SenderPubKey); err != nil {
						body = "(unable to decrypt this letter)"
					} else {
						body = string(plaintext)
					}
					m.app.DecryptedBodies[item.MessageID] = body
				}
				return m, func() tea.Msg {
					return readLetterMsg{item: item, body: body}
				}
			}
		case "r":
			if len(m.items) > 0 {
				item := m.items[m.cursor]
				if item.SenderID == models.SystemUserID {
					break
				}
				// Decrypt and cache the body so compose can show the original letter
				if _, cached := m.app.DecryptedBodies[item.MessageID]; !cached {
					if err := pencrypto.VerifyAndPinKey(item.SenderID.String(), item.SenderPubKey); err != nil {
						m.app.DecryptedBodies[item.MessageID] = fmt.Sprintf("(key verification failed: %v)", err)
					} else if plaintext, err := pencrypto.Decrypt(item.EncryptedBody, m.app.PrivateKey, item.SenderPubKey); err != nil {
						m.app.DecryptedBodies[item.MessageID] = "(unable to decrypt this letter)"
					} else {
						m.app.DecryptedBodies[item.MessageID] = string(plaintext)
					}
				}
				return m, func() tea.Msg {
					return composeToMsg{
						recipientID:    item.SenderID,
						recipientName:  item.SenderName,
						originalMsgID:  item.MessageID,
						originalSender: item.SenderName,
					}
				}
			}
		case "b", "esc":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenHome} }
		}
	case tea.WindowSizeMsg:
		m.viewport.SetWidth(contentWidth())
	case inboxLoadedMsg:
		if msg.append {
			m.items = append(m.items, msg.items...)
			m.loadingMore = false
		} else {
			m.items = msg.items
			m.loading = false
		}
		m.hasMore = msg.hasMore
		if m.cursor >= len(m.items) && m.cursor > 0 {
			m.cursor = len(m.items) - 1
		}
	case errMsg:
		m.err = msg.err.Error()
		m.loading = false
		m.loadingMore = false
	}
	m = m.syncViewport()
	return m, nil
}

func (m InboxModel) syncViewport() InboxModel {
	bh := adaptiveBoxHeight(len(m.items), 6)
	m.viewport.SetHeight(bh - 6)

	var content string
	if m.err != "" {
		content = "\n" + errorStyle.Render(m.err)
	} else if len(m.items) == 0 {
		content = "\n" + mutedStyle.Render("no letters yet")
	} else {
		var b strings.Builder
		for i, item := range m.items {
			prefix := "  "
			if i == m.cursor {
				prefix = "* "
			}
			t := item.DeliveredAt.Local()
			date := fmt.Sprintf("%s %2d %s", t.Format("Jan"), t.Day(), t.Format("3:04pm"))
			isNew := item.ReadAt == nil
			line := fmt.Sprintf("%-14s %s", item.SenderName, date)
			if i == m.cursor {
				line = selectedStyle.Render(prefix + line)
			} else {
				line = menuStyle.Render(prefix + line)
			}
			if isNew {
				line += "  " + newStyle.Render("new")
			}
			b.WriteString(line + "\n")
		}
		content = b.String()
	}

	yOffset := m.viewport.YOffset()
	m.viewport.SetContent(content)
	if len(m.items) > 0 {
		m.viewport.SetYOffset(yOffset)
		// Keep cursor visible
		if m.cursor < m.viewport.YOffset() {
			m.viewport.SetYOffset(m.cursor)
		} else if m.cursor >= m.viewport.YOffset()+m.viewport.Height() {
			m.viewport.SetYOffset(m.cursor - m.viewport.Height() + 1)
		}
	}
	return m
}

func (m InboxModel) View() tea.View {
	title := titleStyle.Render("INBOX")
	header := title + "\n" + divider(contentWidth()) + "\n"
	if m.loading {
		return tea.NewView(emptyScreenView(header, "", "[b] back"))
	}
	if len(m.items) == 0 {
		body := "\n" + mutedStyle.Render("no letters yet")
		if m.err != "" {
			body = "\n" + errorStyle.Render(m.err)
		}
		return tea.NewView(emptyScreenView(header, body, "[b] back"))
	}
	m = m.syncViewport()
	bh := adaptiveBoxHeight(len(m.items), 6)
	var footer string
	if m.confirmDelete && m.cursor < len(m.items) {
		name := m.items[m.cursor].SenderName
		footer = "\n\n" + errorStyle.Render(fmt.Sprintf("delete letter from %s? [y] yes  [n] no", name))
	} else {
		helpText := "[enter] read  [r] reply  [d] delete  [b] back"
		if len(m.items) > 0 && m.items[m.cursor].SenderID == models.SystemUserID {
			helpText = "[enter] read  [d] delete  [b] back"
		}
		footer = "\n\n" + helpStyle.Render(helpText)
	}
	return tea.NewView(screenBox().Height(bh).Render(header + m.viewport.View() + footer))
}

// --- Read Letter ---

type readLetterMsg struct {
	item protocol.InboxItem
	body string // pre-decrypted plaintext
}

type composeToMsg struct {
	recipientID    uuid.UUID
	recipientName  string
	originalMsgID  uuid.UUID // zero = fresh compose
	originalSender string
}

type typewriterTickMsg time.Time

func typewriterDelay(ch rune) time.Duration {
	switch ch {
	case '.', '!', '?':
		return 250 * time.Millisecond
	case ',', ';', ':':
		return 120 * time.Millisecond
	case '\n':
		return 180 * time.Millisecond
	default:
		return 30 * time.Millisecond
	}
}

func typewriterTick(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(t time.Time) tea.Msg { return typewriterTickMsg(t) })
}

// ReadLetterModel displays a single letter.
type ReadLetterModel struct {
	app          *AppState
	item         protocol.InboxItem
	body         string
	viewport     viewport.Model
	err          string
	isContact    bool
	addedContact bool
	// Typewriter state
	runes    []rune
	revealed int
	typing   bool
}

func NewReadLetterModel(app *AppState, item protocol.InboxItem, body string) ReadLetterModel {
	w := contentWidth()
	vp := viewport.New(viewport.WithWidth(w), viewport.WithHeight(viewportHeight()-1))
	vp.SoftWrap = true
	wrapped := wordWrap(body, w)
	m := ReadLetterModel{app: app, item: item, body: wrapped, viewport: vp, isContact: true}
	if item.ReadAt == nil {
		m.runes = []rune(wrapped)
		m.typing = true
		vp.SetContent("\n")
	} else {
		vp.SetContent("\n" + wrapped)
	}
	m.viewport = vp
	return m
}

type readLetterContactsMsg struct {
	contacts []protocol.ContactItem
}

func (m ReadLetterModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		func() tea.Msg {
			m.app.Network.MarkRead(context.Background(), m.item.MessageID)
			return nil
		},
		func() tea.Msg {
			contacts, err := m.app.Network.GetContacts(context.Background())
			if err != nil {
				return nil
			}
			return readLetterContactsMsg{contacts: contacts}
		},
	}
	if m.typing {
		cmds = append(cmds, typewriterTick(29*time.Millisecond))
	}
	return tea.Batch(cmds...)
}

func (m ReadLetterModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	isSystem := m.item.SenderID == models.SystemUserID
	switch msg := msg.(type) {
	case typewriterTickMsg:
		if !m.typing || m.revealed >= len(m.runes) {
			m.typing = false
			return m, nil
		}
		ch := m.runes[m.revealed]
		m.revealed++
		// Skip past whitespace-only runs (e.g. lipgloss padding on blank lines)
		for m.revealed < len(m.runes) && m.runes[m.revealed] == ' ' {
			m.revealed++
		}
		m.viewport.SetContent("\n" + string(m.runes[:m.revealed]))
		m.viewport.GotoBottom()
		if m.revealed >= len(m.runes) {
			m.typing = false
			return m, nil
		}
		return m, typewriterTick(typewriterDelay(ch))
	case tea.KeyPressMsg:
		if m.typing {
			m.typing = false
			m.revealed = len(m.runes)
			m.viewport.SetContent("\n" + m.body)
			m.viewport.GotoBottom()
			return m, nil
		}
		switch msg.String() {
		case "a":
			if !isSystem && !m.isContact && !m.addedContact {
				return m, func() tea.Msg {
					_, err := m.app.Network.AddContactByID(context.Background(), m.item.SenderID)
					if err != nil {
						return errMsg{err: err}
					}
					return contactAddedInReadMsg{}
				}
			}
		case "r":
			if isSystem {
				break
			}
			return m, func() tea.Msg {
				return composeToMsg{
					recipientID:    m.item.SenderID,
					recipientName:  m.item.SenderName,
					originalMsgID:  m.item.MessageID,
					originalSender: m.item.SenderName,
				}
			}
		case "b", "esc":
			return m, func() tea.Msg { return backToInboxMsg{} }
		}
	case readLetterContactsMsg:
		found := false
		for _, c := range msg.contacts {
			if c.UserID == m.item.SenderID {
				found = true
				break
			}
		}
		m.isContact = found
	case contactAddedInReadMsg:
		m.isContact = true
		m.addedContact = true
	case tea.WindowSizeMsg:
		m.viewport.SetWidth(contentWidth())
		m.viewport.SetHeight(viewportHeight() - 1)
	case errMsg:
		m.err = msg.err.Error()
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

type contactAddedInReadMsg struct{}

func (m ReadLetterModel) View() tea.View {
	sentDate := m.item.SentAt.Local().Format("Jan 2 3:04pm")
	arrDate := m.item.DeliveredAt.Local().Format("Jan 2 3:04pm")

	header := fmt.Sprintf("FROM: %s\nSENT: %s  ARRIVED: %s",
		selectedStyle.Render(m.item.SenderName), sentDate, arrDate)
	header += "\n" + divider(contentWidth()) + "\n"

	var help string
	if m.typing {
		help = "press any key to skip"
	} else if m.item.SenderID == models.SystemUserID {
		help = "[b] back"
	} else {
		if m.addedContact {
			help += successStyle.Render("contact added") + "  "
		} else if !m.isContact {
			help += "[a] add contact  "
		}
		help += "[r] reply  [b] back"
	}
	footer := "\n\n" + helpStyle.Render(help)
	return tea.NewView(screenBoxFixed().Render(header + m.viewport.View() + footer))
}

func (n *Network) GetMessage(ctx context.Context, msgID uuid.UUID) (*protocol.GetMessageResponse, error) {
	resp, err := n.Send(ctx, protocol.MsgGetMessage, protocol.GetMessageRequest{MessageID: msgID})
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(resp.Payload)
	var result protocol.GetMessageResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing message response: %w", err)
	}
	return &result, nil
}

func (n *Network) GetPublicKeyForUser(ctx context.Context, userID uuid.UUID) ([]byte, error) {
	resp, err := n.Send(ctx, protocol.MsgGetPublicKey, protocol.GetPublicKeyRequest{UserID: userID})
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(resp.Payload)
	var result protocol.PublicKeyResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing public key response: %w", err)
	}
	if len(result.PublicKey) == 0 {
		return nil, fmt.Errorf("empty public key returned")
	}
	// Verify against locally pinned key (TOFU model).
	// On first contact, the key is pinned. On subsequent contacts, if the key
	// has changed, this returns an error to prevent MITM attacks by the server.
	if err := pencrypto.VerifyAndPinKey(userID.String(), result.PublicKey); err != nil {
		return nil, fmt.Errorf("key verification failed for %s: %w", userID, err)
	}
	return result.PublicKey, nil
}
