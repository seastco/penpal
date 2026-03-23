package client

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	pencrypto "github.com/seastco/penpal/internal/crypto"
)

// TUI is the root bubbletea model that orchestrates screen switching.
type TUI struct {
	app             *AppState
	currentModel    tea.Model
	screen          Screen
	cachedInbox     *InboxModel     // preserved when entering a letter, restored on back
	readingMsgIdx   int             // index of the letter currently being read
	cachedSent      *SentModel      // preserved when entering tracking from sent
	cachedInTransit *InTransitModel // preserved when entering tracking from in-transit
}

// NewTUI creates the root TUI model.
func NewTUI(app *AppState) TUI {
	// Load saved theme
	if dir, err := pencrypto.PenpalDir(); err == nil {
		app.ThemeName = loadTheme(dir)
	}

	var startModel tea.Model
	var startScreen Screen

	if app.UserID.String() == "00000000-0000-0000-0000-000000000000" {
		// Not registered yet
		startModel = NewRegisterModel(app)
		startScreen = ScreenRegister
	} else if PinFileExists() {
		startModel = NewPinEntryModel(app)
		startScreen = ScreenPinEntry
	} else {
		startModel = NewHomeModel(app)
		startScreen = ScreenHome
	}

	return TUI{
		app:          app,
		currentModel: startModel,
		screen:       startScreen,
	}
}

func (t TUI) Init() tea.Cmd {
	title := "Penpal"
	if t.app.Username != "" {
		title = fmt.Sprintf("Penpal — %s", t.app.Address())
	}
	return tea.Batch(
		t.currentModel.Init(),
		tea.SetWindowTitle(title),
	)
}

func (t TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Always track terminal size
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		termWidth = ws.Width
		termHeight = ws.Height
	}

	switch msg := msg.(type) {
	case switchScreenMsg:
		return t.switchTo(msg.screen)
	case readLetterMsg:
		// Snapshot the inbox before switching so 'b' restores it instantly
		if inbox, ok := t.currentModel.(InboxModel); ok {
			t.cachedInbox = &inbox
			t.readingMsgIdx = inbox.cursor
		}
		m := NewReadLetterModel(t.app, msg.item, msg.body)
		t.currentModel = m
		t.screen = ScreenReadLetter
		return t, m.Init()
	case backToInboxMsg:
		if t.cachedInbox != nil {
			inbox := *t.cachedInbox
			// Mark the letter we just read so the "new" badge disappears
			if t.readingMsgIdx >= 0 && t.readingMsgIdx < len(inbox.items) && inbox.items[t.readingMsgIdx].ReadAt == nil {
				now := time.Now()
				inbox.items[t.readingMsgIdx].ReadAt = &now
			}
			inbox = inbox.syncViewport()
			t.currentModel = inbox
			t.screen = ScreenInbox
			t.cachedInbox = nil
			return t, nil
		}
		return t.switchTo(ScreenInbox)
	case composeToMsg:
		if inbox, ok := t.currentModel.(InboxModel); ok {
			t.cachedInbox = &inbox
		}
		m := NewComposeModelTo(t.app, msg.recipientID, msg.recipientName, msg.originalMsgID, msg.originalSender)
		t.currentModel = m
		t.screen = ScreenCompose
		return t, m.Init()
	case trackLetterMsg:
		if sent, ok := t.currentModel.(SentModel); ok {
			t.cachedSent = &sent
		}
		if inTransit, ok := t.currentModel.(InTransitModel); ok {
			t.cachedInTransit = &inTransit
		}
		m := NewTrackingModel(t.app, msg.msgID, msg.label, msg.origin)
		t.currentModel = m
		t.screen = ScreenTracking
		return t, m.Init()
	}

	var cmd tea.Cmd
	t.currentModel, cmd = t.currentModel.Update(msg)
	return t, cmd
}

func (t TUI) switchTo(screen Screen) (tea.Model, tea.Cmd) {
	prevScreen := t.screen
	t.screen = screen
	var m tea.Model
	switch screen {
	case ScreenHome:
		m = NewHomeModel(t.app)
	case ScreenRegister:
		m = NewRegisterModel(t.app)
	case ScreenInbox:
		if t.cachedInbox != nil {
			inbox := *t.cachedInbox
			t.cachedInbox = nil
			t.currentModel = inbox
			t.screen = screen
			return t, nil
		}
		m = NewInboxModel(t.app)
	case ScreenInTransit:
		if t.cachedInTransit != nil {
			inTransit := *t.cachedInTransit
			t.cachedInTransit = nil
			t.currentModel = inTransit
			t.screen = screen
			return t, nil
		}
		m = NewInTransitModel(t.app)
	case ScreenSent:
		if t.cachedSent != nil {
			sent := *t.cachedSent
			t.cachedSent = nil
			t.currentModel = sent
			t.screen = screen
			return t, nil
		}
		m = NewSentModel(t.app)
	case ScreenCompose:
		m = NewComposeModel(t.app)
	case ScreenAddressBook:
		m = NewAddressBookModel(t.app)
	case ScreenAddContact:
		m = NewAddContactModel(t.app)
	case ScreenStamps:
		m = NewStampsModel(t.app)
	case ScreenSplash:
		m = NewSplashModel(t.app)
	case ScreenPinEntry:
		m = NewPinEntryModel(t.app)
	case ScreenPinSetup:
		if prevScreen == ScreenRegister || prevScreen == ScreenRegisterCity {
			m = NewPinSetupModelWithOrigin(t.app, ScreenHome)
		} else {
			m = NewPinSetupModel(t.app)
		}
	case ScreenSettings:
		m = NewSettingsModel(t.app)
	default:
		m = NewHomeModel(t.app)
	}
	t.currentModel = m
	return t, m.Init()
}

func (t TUI) View() string {
	return centeredView(t.currentModel.View())
}
