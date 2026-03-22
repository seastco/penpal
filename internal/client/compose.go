package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	pencrypto "github.com/seastco/penpal/internal/crypto"
	"github.com/seastco/penpal/internal/models"
	"github.com/seastco/penpal/internal/protocol"
)

// ComposeModel handles the letter composition flow.
type ComposeModel struct {
	app  *AppState
	step int // 0=recipient, 1=body, 2=shipping, 3=stamps

	// Recipient
	recipientInput textinput.Model
	contacts       []protocol.ContactItem
	filteredIdx    []int
	recipientSel   int
	recipientID    uuid.UUID
	recipientName  string

	// Body
	bodyArea textarea.Model

	// Shipping
	shippingInfo *protocol.ShippingInfoResponse
	shippingIdx  int

	// Stamp selection (multi-select)
	stamps         []models.Stamp
	stampCursor    int
	selectedStamps map[uuid.UUID]bool
	requiredStamps int // how many stamps the chosen tier needs

	err           string
	sending       bool
	draftRestored bool

	// Reply reference (set only when replying to a letter)
	originalMsgID    uuid.UUID
	originalSender   string
	showingOriginal  bool
	originalViewport viewport.Model

	origin Screen // screen to return to on Esc
}

func NewComposeModel(app *AppState) ComposeModel {
	ri := textinput.New()
	ri.Placeholder = "contact name"
	ri.Focus()
	ri.CharLimit = 32
	ri.Width = contentWidth() - 8

	ta := textarea.New()
	ta.Placeholder = "Write your letter..."
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.SetWidth(contentWidth())
	ta.SetHeight(10)
	ta.CharLimit = 0 // no char limit; word limit enforced on send

	return ComposeModel{
		app:            app,
		recipientInput: ri,
		bodyArea:       ta,
		shippingIdx:    0,
		selectedStamps: make(map[uuid.UUID]bool),
		origin:         ScreenHome,
	}
}

// NewComposeModelTo creates a compose model pre-filled with a recipient.
// When originalMsgID is non-zero, the compose screen supports toggling to view the original letter.
func NewComposeModelTo(app *AppState, recipientID uuid.UUID, recipientName string, originalMsgID uuid.UUID, originalSender string) ComposeModel {
	m := NewComposeModel(app)
	m.recipientID = recipientID
	m.recipientName = recipientName
	m.origin = ScreenInbox
	m.step = 1
	m.recipientInput.Blur()
	m.bodyArea.Focus()

	if originalMsgID != uuid.Nil {
		m.originalMsgID = originalMsgID
		m.originalSender = originalSender

		vp := viewport.New(contentWidth(), viewportHeight()-2)
		vp.KeyMap = viewport.DefaultKeyMap()
		body, ok := app.DecryptedBodies[originalMsgID]
		if !ok {
			body = "(original letter unavailable)"
		}
		vp.SetContent(body)
		m.originalViewport = vp
	}

	m.loadDraftIfExists()

	return m
}

type contactsLoadedMsg struct {
	contacts []protocol.ContactItem
}

type shippingLoadedMsg struct {
	info *protocol.ShippingInfoResponse
}

type letterSentMsg struct {
	resp *protocol.LetterSentResponse
}

func (m ComposeModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, func() tea.Msg {
		contacts, err := m.app.Network.GetContacts(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		return contactsLoadedMsg{contacts: contacts}
	})
}

func (m ComposeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		m.bodyArea.SetWidth(contentWidth())
		m.recipientInput.Width = contentWidth() - 8
		if m.originalMsgID != uuid.Nil {
			m.originalViewport.Width = contentWidth()
			m.originalViewport.Height = viewportHeight() - 2
		}
	}

	// Handle contacts loaded at top level so it works regardless of step
	if cl, ok := msg.(contactsLoadedMsg); ok {
		m.contacts = cl.contacts
		if m.step == 0 {
			m.filterContacts()
		}
		return m, nil
	}

	switch m.step {
	case 0:
		return m.updateRecipient(msg)
	case 1:
		return m.updateBody(msg)
	case 2:
		return m.updateShipping(msg)
	case 3:
		return m.updateStamp(msg)
	}
	return m, nil
}

func (m ComposeModel) updateRecipient(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Still loading — only allow back
		if m.contacts == nil {
			if msg.String() == "b" || msg.String() == "esc" {
				return m, func() tea.Msg { return switchScreenMsg{screen: m.origin} }
			}
			return m, nil
		}
		// Empty contacts state: simple key handling
		if len(m.contacts) == 0 {
			switch msg.String() {
			case "a":
				return m, func() tea.Msg { return switchScreenMsg{screen: ScreenAddContact} }
			case "b", "esc":
				return m, func() tea.Msg { return switchScreenMsg{screen: m.origin} }
			}
			return m, nil
		}
		switch msg.String() {
		case "enter":
			if len(m.filteredIdx) > 0 && m.recipientSel < len(m.filteredIdx) {
				c := m.contacts[m.filteredIdx[m.recipientSel]]
				m.recipientID = c.UserID
				m.recipientName = c.Username
				m.step = 1
				m.recipientInput.Blur()
				m.bodyArea.Focus()
				m.loadDraftIfExists()
				return m, nil
			}
		case "up":
			if m.recipientSel > 0 {
				m.recipientSel--
			}
		case "down":
			if m.recipientSel < len(m.filteredIdx)-1 {
				m.recipientSel++
			}
		case "ctrl+b", "esc":
			return m, func() tea.Msg { return switchScreenMsg{screen: m.origin} }
		default:
			var cmd tea.Cmd
			m.recipientInput, cmd = m.recipientInput.Update(msg)
			m.filterContacts()
			return m, cmd
		}
	case errMsg:
		m.err = msg.err.Error()
	}
	return m, nil
}

func (m *ComposeModel) filterContacts() {
	query := strings.ToLower(m.recipientInput.Value())
	if query == "" {
		m.filteredIdx = nil
		for i := range m.contacts {
			m.filteredIdx = append(m.filteredIdx, i)
		}
		m.recipientSel = 0
		return
	}

	type scored struct {
		idx   int
		score int
	}
	var results []scored
	for i, c := range m.contacts {
		name := strings.ToLower(c.Username)
		city := strings.ToLower(c.HomeCity)
		s := 0
		if strings.HasPrefix(name, query) {
			s = 100
		} else if strings.Contains(name, query) {
			s = 50
		} else if strings.Contains(city, query) {
			s = 25
		} else if fuzzyMatch(name, query) {
			s = 10
		}
		if s > 0 {
			results = append(results, scored{idx: i, score: s})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	m.filteredIdx = nil
	for _, r := range results {
		m.filteredIdx = append(m.filteredIdx, r.idx)
	}
	m.recipientSel = 0
}

// fuzzyMatch checks if query is a subsequence of s.
func fuzzyMatch(s, query string) bool {
	qi := 0
	for i := 0; i < len(s) && qi < len(query); i++ {
		if s[i] == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}

func (m ComposeModel) updateBody(msg tea.Msg) (tea.Model, tea.Cmd) {
	// When showing the original letter, only handle toggle-back and scroll keys
	if m.showingOriginal {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "ctrl+r":
				m.showingOriginal = false
				m.bodyArea.Focus()
				return m, nil
			case "up", "down", "pgup", "pgdown", "home", "end":
				var cmd tea.Cmd
				m.originalViewport, cmd = m.originalViewport.Update(msg)
				return m, cmd
			}
			// Swallow all other keys while viewing original
			return m, nil
		}
		// Forward non-key messages (e.g. WindowSizeMsg) normally
		var cmd tea.Cmd
		m.originalViewport, cmd = m.originalViewport.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.draftRestored = false
		switch msg.String() {
		case "ctrl+r":
			if m.originalMsgID != uuid.Nil {
				m.showingOriginal = true
				m.bodyArea.Blur()
				return m, nil
			}
		case "ctrl+s":
			if len(strings.Fields(m.bodyArea.Value())) > 5000 {
				m.err = "letter exceeds 5,000 word limit"
				return m, nil
			}
			// Move to shipping — fetch stamps + shipping info in parallel
			m.step = 2
			m.bodyArea.Blur()
			return m, tea.Batch(m.fetchStamps(), m.fetchShipping())
		case "ctrl+b", "esc":
			m.saveDraft()
			return m, func() tea.Msg { return switchScreenMsg{screen: m.origin} }
		}
	case errMsg:
		m.err = msg.err.Error()
	}
	var cmd tea.Cmd
	m.bodyArea, cmd = m.bodyArea.Update(msg)
	return m, cmd
}

func (m ComposeModel) fetchShipping() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		resp, err := m.app.Network.Send(ctx, protocol.MsgGetShipping, protocol.GetShippingRequest{
			RecipientID: m.recipientID,
		})
		if err != nil {
			return errMsg{err: err}
		}
		data, _ := json.Marshal(resp.Payload)
		var info protocol.ShippingInfoResponse
		if err := json.Unmarshal(data, &info); err != nil {
			return errMsg{err: fmt.Errorf("parsing shipping info: %w", err)}
		}
		return shippingLoadedMsg{info: &info}
	}
}

func (m ComposeModel) fetchStamps() tea.Cmd {
	return func() tea.Msg {
		stamps, err := m.app.Network.GetStampsAll(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		return composeStampsLoadedMsg{stamps: stamps}
	}
}

type composeStampsLoadedMsg struct {
	stamps []models.Stamp
}

func (m ComposeModel) updateShipping(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.shippingIdx > 0 {
				m.shippingIdx--
			}
		case "down", "j":
			opts := m.shippingOptions()
			if m.shippingIdx < len(opts)-1 {
				m.shippingIdx++
			}
		case "enter":
			opts := m.shippingOptions()
			if m.shippingIdx < len(opts) {
				opt := opts[m.shippingIdx]
				if opt.locked {
					m.err = fmt.Sprintf("need %d stamps for %s (you have %d)", opt.stampsReq, opt.name, len(m.stamps))
					return m, nil
				}
				m.requiredStamps = opt.stampsReq
				m.selectedStamps = make(map[uuid.UUID]bool)
				m.stampCursor = 0
				m.step = 3
				m.err = ""
			}
		case "b", "esc":
			m.step = 1
			m.bodyArea.Focus()
			m.shippingIdx = 0
		}
	case shippingLoadedMsg:
		m.shippingInfo = msg.info
	case composeStampsLoadedMsg:
		m.stamps = filterAndSortStamps(msg.stamps)
	case errMsg:
		m.err = msg.err.Error()
	}
	return m, nil
}

func filterAndSortStamps(stamps []models.Stamp) []models.Stamp {
	var filtered []models.Stamp
	for _, s := range stamps {
		t := s.StampType
		switch {
		case strings.HasPrefix(t, "common:"):
			if _, ok := stampEmoji[t]; ok {
				filtered = append(filtered, s)
			}
		case strings.HasPrefix(t, "state:"), strings.HasPrefix(t, "country:"):
			filtered = append(filtered, s)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		ci, cj := stampCategoryOrder(filtered[i].StampType), stampCategoryOrder(filtered[j].StampType)
		if ci != cj {
			return ci < cj
		}
		return filtered[i].StampType < filtered[j].StampType
	})
	return filtered
}

func (m ComposeModel) updateStamp(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.stampCursor > 0 {
				m.stampCursor--
			}
		case "down", "j":
			if m.stampCursor < len(m.stamps)-1 {
				m.stampCursor++
			}
		case " ", "enter":
			if len(m.stamps) > 0 && m.stampCursor < len(m.stamps) {
				s := m.stamps[m.stampCursor]
				if m.selectedStamps[s.ID] {
					// Deselect
					delete(m.selectedStamps, s.ID)
				} else if len(m.selectedStamps) < m.requiredStamps {
					// Select
					m.selectedStamps[s.ID] = true
				}
				m.err = ""
			}
		case "ctrl+s":
			// Send when exact count selected
			if len(m.selectedStamps) == m.requiredStamps {
				if m.sending {
					return m, nil
				}
				m.sending = true
				return m, m.sendLetter()
			}
			m.err = fmt.Sprintf("select %d stamp(s) to send", m.requiredStamps)
		case "b", "esc":
			m.step = 2
			m.selectedStamps = make(map[uuid.UUID]bool)
			m.err = ""
		}
	case letterSentMsg:
		_ = DeleteDraft(m.recipientID)
		return m, func() tea.Msg {
			return trackLetterMsg{msgID: msg.resp.MessageID, label: "to " + m.recipientName, origin: ScreenHome}
		}
	case errMsg:
		m.err = msg.err.Error()
		m.sending = false
	}
	return m, nil
}

func (m ComposeModel) sendLetter() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Get recipient's public key
		pubKeyBytes, err := m.app.Network.GetPublicKeyForUser(ctx, m.recipientID)
		if err != nil {
			return errMsg{err: fmt.Errorf("getting recipient key: %w", err)}
		}

		body := m.bodyArea.Value()
		if strings.TrimSpace(body) == "" {
			return errMsg{err: fmt.Errorf("letter body is empty")}
		}

		// Encrypt the letter
		encrypted, err := pencrypto.Encrypt([]byte(body), m.app.PrivateKey, pubKeyBytes)
		if err != nil {
			return errMsg{err: fmt.Errorf("encrypting letter: %w", err)}
		}

		allTiers := models.AllTiers()
		tier := string(allTiers[0])
		if m.shippingIdx >= 0 && m.shippingIdx < len(allTiers) {
			tier = string(allTiers[m.shippingIdx])
		}

		var stampIDs []uuid.UUID
		for id := range m.selectedStamps {
			stampIDs = append(stampIDs, id)
		}

		resp, err := m.app.Network.SendLetter(ctx, protocol.SendLetterRequest{
			RecipientID:   m.recipientID,
			EncryptedBody: encrypted,
			ShippingTier:  tier,
			StampIDs:      stampIDs,
		})
		if err != nil {
			return errMsg{err: err}
		}
		return letterSentMsg{resp: resp}
	}
}

func (m ComposeModel) saveDraft() {
	if m.recipientID == uuid.Nil {
		return
	}
	body := strings.TrimSpace(m.bodyArea.Value())
	if body == "" {
		_ = DeleteDraft(m.recipientID)
		return
	}
	_ = SaveDraft(Draft{
		RecipientID:    m.recipientID,
		RecipientName:  m.recipientName,
		Body:           m.bodyArea.Value(),
		OriginalMsgID:  m.originalMsgID,
		OriginalSender: m.originalSender,
		SavedAt:        time.Now(),
	})
}

func (m *ComposeModel) loadDraftIfExists() {
	if m.recipientID == uuid.Nil {
		return
	}
	d, err := LoadDraft(m.recipientID)
	if err != nil || d == nil {
		return
	}
	m.bodyArea.SetValue(d.Body)
	if m.originalMsgID == uuid.Nil && d.OriginalMsgID != uuid.Nil {
		m.originalMsgID = d.OriginalMsgID
		m.originalSender = d.OriginalSender
	}
	m.draftRestored = true
}

func (m ComposeModel) View() string {
	switch m.step {
	case 0:
		return m.viewRecipient()
	case 1:
		return m.viewBody()
	case 2:
		return m.viewShipping()
	case 3:
		return m.viewStamp()
	}
	return ""
}

func (m ComposeModel) viewRecipient() string {
	title := titleStyle.Render("COMPOSE")
	header := title + "\n" + divider(contentWidth()) + "\n"

	// Still loading contacts
	if m.contacts == nil {
		body := "\n" + mutedStyle.Render("loading...")
		return emptyScreenView(header, body, "[b] back")
	}

	// Empty contacts state
	if len(m.contacts) == 0 {
		body := "\n" + mutedStyle.Render("no contacts yet")
		return emptyScreenView(header, body, "[a] add contact  [b] back")
	}

	content := header
	content += fmt.Sprintf("to: %s\n\n", m.recipientInput.View())

	maxVisible := viewportHeight() - 6 // dynamic based on terminal height
	if maxVisible < 4 {
		maxVisible = 4
	}
	start := 0
	if m.recipientSel >= maxVisible {
		start = m.recipientSel - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(m.filteredIdx) {
		end = len(m.filteredIdx)
	}
	for i := start; i < end; i++ {
		c := m.contacts[m.filteredIdx[i]]
		prefix := "    "
		if i == m.recipientSel {
			prefix = "> "
		}
		line := fmt.Sprintf("%-14s %s", c.Username, mutedStyle.Render(c.HomeCity))
		if i == m.recipientSel {
			content += selectedStyle.Render(prefix+line) + "\n"
		} else {
			content += "  " + line + "\n"
		}
	}

	if m.err != "" {
		content += "\n" + errorStyle.Render(m.err) + "\n"
	}
	content += "\n" + helpStyle.Render("[enter] select  [ctrl+b] back")
	return screenBox().Render(content)
}

func (m ComposeModel) viewBody() string {
	if m.showingOriginal {
		return m.viewOriginal()
	}

	title := titleStyle.Render("COMPOSE")
	content := title + "\n" + divider(contentWidth()) + "\n"
	content += fmt.Sprintf("to: %s\n", selectedStyle.Render(m.recipientName))
	content += divider(contentWidth()) + "\n"
	content += m.bodyArea.View() + "\n"

	words := len(strings.Fields(m.bodyArea.Value()))
	var wordLine string
	if words >= 4000 {
		wordLine = fmt.Sprintf("%d / 5,000 words", words)
	} else {
		wordLine = fmt.Sprintf("%d words", words)
	}
	if m.draftRestored {
		wordLine += "  (draft restored)"
	}
	if words >= 5000 {
		content += "\n" + errorStyle.Render(wordLine)
	} else {
		content += "\n" + mutedStyle.Render(wordLine)
	}

	if m.err != "" {
		content += "\n" + errorStyle.Render(m.err)
	}
	help := "[ctrl+s] send"
	if m.originalMsgID != uuid.Nil {
		help += "  [ctrl+r] view letter"
	}
	help += "  [ctrl+b] back"
	content += "\n" + helpStyle.Render(help)
	return screenBox().Render(content)
}

func (m ComposeModel) viewOriginal() string {
	title := titleStyle.Render("ORIGINAL LETTER")
	content := title + "\n" + divider(contentWidth()) + "\n"
	content += fmt.Sprintf("from: %s\n", selectedStyle.Render(m.originalSender))
	content += divider(contentWidth()) + "\n"
	content += m.originalViewport.View() + "\n"
	content += "\n" + helpStyle.Render("[ctrl+r] back to reply")
	return screenBoxFixed().Render(content)
}

func stampDisplayName(stampType string) string {
	if strings.HasPrefix(stampType, "common:") {
		if stampType == "common:flag" {
			return "USA"
		}
		return commonDisplayName(stampType)
	}
	if strings.HasPrefix(stampType, "rare:") {
		return rareDisplayName(stampType)
	}
	if strings.HasPrefix(stampType, "state:") {
		code := strings.ToUpper(strings.TrimPrefix(stampType, "state:"))
		if name, ok := stateNames[code]; ok {
			return fmt.Sprintf("%s (%s)", name, code)
		}
		return code
	}
	if strings.HasPrefix(stampType, "country:") {
		code := strings.ToUpper(strings.TrimPrefix(stampType, "country:"))
		return code
	}
	return stampType
}

func stampEmojiFor(stampType string) string {
	if e, ok := stampEmoji[stampType]; ok {
		return e
	}
	if strings.HasPrefix(stampType, "state:") {
		code := strings.ToUpper(strings.TrimPrefix(stampType, "state:"))
		if e, ok := stateEmoji[code]; ok {
			return e
		}
		return "📍"
	}
	if strings.HasPrefix(stampType, "country:") {
		code := strings.ToUpper(strings.TrimPrefix(stampType, "country:"))
		if e, ok := countryEmoji[code]; ok {
			return e
		}
		return "🌍"
	}
	return "📮"
}

func stampCategoryOrder(stampType string) int {
	switch {
	case strings.HasPrefix(stampType, "common:"):
		return 0
	case strings.HasPrefix(stampType, "rare:"):
		return 1
	case strings.HasPrefix(stampType, "state:"):
		return 2
	case strings.HasPrefix(stampType, "country:"):
		return 3
	default:
		return 4
	}
}

func stampCategoryLabel(stampType string) string {
	switch {
	case strings.HasPrefix(stampType, "common:"):
		return "common"
	case strings.HasPrefix(stampType, "rare:"):
		return "rare"
	case strings.HasPrefix(stampType, "state:"):
		return "state"
	case strings.HasPrefix(stampType, "country:"):
		return "country"
	default:
		return "unknown"
	}
}

func (m ComposeModel) viewShipping() string {
	title := titleStyle.Render("SHIPPING")
	content := title + "\n" + divider(contentWidth()) + "\n"

	content += fmt.Sprintf("to: %s\n", selectedStyle.Render(m.recipientName))
	if m.shippingInfo != nil {
		content += fmt.Sprintf("%s -> %s\n", m.shippingInfo.FromCity, m.shippingInfo.ToCity)
		if len(m.shippingInfo.Options) > 0 {
			opt := m.shippingInfo.Options[0]
			content += fmt.Sprintf("distance: %.0f mi (%d hops)\n", opt.Distance, opt.Hops)
		}
	}

	content += "\n"
	opts := m.shippingOptions()
	for i, opt := range opts {
		prefix := "  "
		if i == m.shippingIdx {
			prefix = "> "
		}

		stampLabel := fmt.Sprintf("(%d stamp)", opt.stampsReq)
		if opt.stampsReq > 1 {
			stampLabel = fmt.Sprintf("(%d stamps)", opt.stampsReq)
		}

		est := "..."
		if opt.estDelivery != "" {
			est = "~" + opt.estDelivery
		}
		line := fmt.Sprintf("[%d] %-14s%-12s %s", i+1, opt.name, est, stampLabel)
		if i == m.shippingIdx {
			if opt.locked {
				content += mutedStyle.Render("🔒"+prefix+line) + "\n"
			} else {
				content += selectedStyle.Render(prefix+line) + "\n"
			}
		} else {
			if opt.locked {
				content += mutedStyle.Render("🔒 "+line) + "\n"
			} else {
				content += "  " + line + "\n"
			}
		}
	}

	if m.err != "" {
		content += "\n" + errorStyle.Render(m.err) + "\n"
	}
	content += "\n" + helpStyle.Render("[enter] select  [b] back")
	return screenBox().Render(content)
}

func (m ComposeModel) viewStamp() string {
	selected := len(m.selectedStamps)
	title := titleStyle.Render(fmt.Sprintf("STAMPS (%d/%d)", selected, m.requiredStamps))
	content := title + "\n" + divider(contentWidth()) + "\n"

	tierName := models.AllTiers()[m.shippingIdx].DisplayName()
	content += fmt.Sprintf("to: %s · %s\n", selectedStyle.Render(m.recipientName), mutedStyle.Render(strings.ToLower(tierName)))

	if len(m.stamps) == 0 {
		content += "\n" + mutedStyle.Render("no stamps available") + "\n"
		content += "\n" + helpStyle.Render("[b] back")
		return screenBox().Render(content)
	}

	content += fmt.Sprintf("\nselect %d stamp(s) for your letter:\n\n", m.requiredStamps)

	maxVisible := 8
	start := 0
	if m.stampCursor >= maxVisible {
		start = m.stampCursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(m.stamps) {
		end = len(m.stamps)
	}
	for i := start; i < end; i++ {
		s := m.stamps[i]
		prefix := "  "
		if i == m.stampCursor {
			prefix = "> "
		}

		check := "[ ]"
		if m.selectedStamps[s.ID] {
			check = "[*]"
		}

		emoji := stampEmojiFor(s.StampType)
		name := stampDisplayName(s.StampType)
		rarity := mutedStyle.Render(stampCategoryLabel(s.StampType))
		line := fmt.Sprintf("%s %s  %-20s %s", check, emoji, name, rarity)
		if i == m.stampCursor {
			content += selectedStyle.Render(prefix+line) + "\n"
		} else {
			content += prefix + line + "\n"
		}
	}

	if m.sending {
		content += "\n" + mutedStyle.Render("sending...") + "\n"
	}
	if m.err != "" {
		content += "\n" + errorStyle.Render(m.err) + "\n"
	}

	help := "[space] toggle"
	if selected == m.requiredStamps {
		help += "  [ctrl+s] send"
	}
	help += "  [b] back"
	content += "\n" + helpStyle.Render(help)
	return screenBox().Render(content)
}

type shippingOpt struct {
	name        string
	estDelivery string // "Mon Jan 5" or empty if loading
	stampsReq   int
	locked      bool
}

func (m ComposeModel) shippingOptions() []shippingOpt {
	tiers := models.AllTiers()
	opts := make([]shippingOpt, len(tiers))
	for i, tier := range tiers {
		req := tier.StampsRequired()
		opts[i] = shippingOpt{
			name:      tier.DisplayName(),
			stampsReq: req,
			locked:    len(m.stamps) < req,
		}
	}
	if m.shippingInfo != nil && len(m.shippingInfo.Options) == len(opts) {
		for i, info := range m.shippingInfo.Options {
			if !info.EstDelivery.IsZero() {
				opts[i].estDelivery = info.EstDelivery.Format("Mon Jan 2")
			}
		}
	}
	return opts
}
