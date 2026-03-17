package client

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/stove/penpal/internal/protocol"
)

func testApp() *AppState {
	return &AppState{
		UserID:          uuid.New(),
		Username:        "testuser",
		Discriminator:   "1234",
		DecryptedBodies: make(map[uuid.UUID]string),
	}
}

func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func ctrlKeyMsg(key tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: key}
}

// --- Home Screen Tests ---

func TestHome_NavigationKeys(t *testing.T) {
	tests := []struct {
		key    string
		expect Screen
	}{
		{"i", ScreenInbox},
		{"s", ScreenSent},
		{"c", ScreenCompose},
		{"m", ScreenStamps},
		{"a", ScreenAddressBook},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			m := NewHomeModel(testApp())
			_, cmd := m.Update(keyMsg(tt.key))
			if cmd == nil {
				t.Fatal("expected command, got nil")
			}
			msg := cmd()
			sm, ok := msg.(switchScreenMsg)
			if !ok {
				t.Fatalf("expected switchScreenMsg, got %T", msg)
			}
			if sm.screen != tt.expect {
				t.Errorf("expected screen %d, got %d", tt.expect, sm.screen)
			}
		})
	}
}

func TestHome_QuitKey(t *testing.T) {
	m := NewHomeModel(testApp())
	_, cmd := m.Update(keyMsg("q"))
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestHome_ViewContainsMenuItems(t *testing.T) {
	m := NewHomeModel(testApp())
	view := m.View()
	for _, item := range []string{"Inbox", "Sent", "Compose", "Stamps", "Address Book"} {
		if !strings.Contains(view, item) {
			t.Errorf("view missing menu item: %s", item)
		}
	}
}

func TestHome_ViewShowsAddress(t *testing.T) {
	app := testApp()
	m := NewHomeModel(app)
	view := m.View()
	if !strings.Contains(view, "testuser#1234") {
		t.Error("view should show user address")
	}
}

func TestHome_DataRefresh(t *testing.T) {
	m := NewHomeModel(testApp())
	updated, _ := m.Update(dataRefreshMsg{inboxNew: 5, transitCount: 3})
	hm := updated.(HomeModel)
	if hm.inboxNew != 5 || hm.transitN != 3 {
		t.Errorf("expected 5 new / 3 transit, got %d / %d", hm.inboxNew, hm.transitN)
	}
}

// --- Inbox Tests ---

func TestInbox_CursorNavigation(t *testing.T) {
	m := NewInboxModel(testApp())
	m.items = []protocol.InboxItem{
		{SenderName: "alice", DeliveredAt: time.Now()},
		{SenderName: "bob", DeliveredAt: time.Now()},
		{SenderName: "carol", DeliveredAt: time.Now()},
	}
	m.loading = false

	// Move down
	updated, _ := m.Update(keyMsg("j"))
	im := updated.(InboxModel)
	if im.cursor != 1 {
		t.Errorf("cursor should be 1 after down, got %d", im.cursor)
	}

	// Move down again
	updated, _ = im.Update(keyMsg("j"))
	im = updated.(InboxModel)
	if im.cursor != 2 {
		t.Errorf("cursor should be 2, got %d", im.cursor)
	}

	// Down at bottom shouldn't overflow
	updated, _ = im.Update(keyMsg("j"))
	im = updated.(InboxModel)
	if im.cursor != 2 {
		t.Errorf("cursor should stay at 2, got %d", im.cursor)
	}

	// Move up
	updated, _ = im.Update(keyMsg("k"))
	im = updated.(InboxModel)
	if im.cursor != 1 {
		t.Errorf("cursor should be 1 after up, got %d", im.cursor)
	}
}

func TestInbox_EnterOpensLetter(t *testing.T) {
	m := NewInboxModel(testApp())
	m.items = []protocol.InboxItem{
		{SenderName: "alice", SenderID: uuid.New(), DeliveredAt: time.Now()},
	}
	m.loading = false

	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("expected read letter command")
	}
	msg := cmd()
	rl, ok := msg.(readLetterMsg)
	if !ok {
		t.Fatalf("expected readLetterMsg, got %T", msg)
	}
	if rl.item.SenderName != "alice" {
		t.Errorf("expected alice, got %s", rl.item.SenderName)
	}
}

func TestInbox_ReplyKey(t *testing.T) {
	senderID := uuid.New()
	m := NewInboxModel(testApp())
	m.items = []protocol.InboxItem{
		{SenderName: "alice", SenderID: senderID, DeliveredAt: time.Now()},
	}
	m.loading = false

	_, cmd := m.Update(keyMsg("r"))
	if cmd == nil {
		t.Fatal("expected compose command")
	}
	msg := cmd()
	ct, ok := msg.(composeToMsg)
	if !ok {
		t.Fatalf("expected composeToMsg, got %T", msg)
	}
	if ct.recipientID != senderID {
		t.Error("reply should target the sender")
	}
}

func TestInbox_BackGoesHome(t *testing.T) {
	for _, key := range []string{"b", "esc"} {
		t.Run(key, func(t *testing.T) {
			m := NewInboxModel(testApp())
			_, cmd := m.Update(keyMsg(key))
			if cmd == nil {
				t.Fatal("expected command")
			}
			msg := cmd()
			sm, ok := msg.(switchScreenMsg)
			if !ok {
				t.Fatalf("expected switchScreenMsg, got %T", msg)
			}
			if sm.screen != ScreenHome {
				t.Errorf("expected home screen, got %d", sm.screen)
			}
		})
	}
}

func TestInbox_LoadedMsg(t *testing.T) {
	m := NewInboxModel(testApp())
	if !m.loading {
		t.Error("should start loading")
	}
	items := []protocol.InboxItem{
		{SenderName: "alice", DeliveredAt: time.Now()},
	}
	updated, _ := m.Update(inboxLoadedMsg{items: items})
	im := updated.(InboxModel)
	if im.loading {
		t.Error("should not be loading after data received")
	}
	if len(im.items) != 1 {
		t.Errorf("expected 1 item, got %d", len(im.items))
	}
}

func TestInbox_EmptyView(t *testing.T) {
	m := NewInboxModel(testApp())
	m.loading = false
	m.items = nil
	view := m.View()
	if !strings.Contains(view, "no letters yet") {
		t.Error("empty inbox should show 'no letters yet'")
	}
}

func TestInbox_NewBadge(t *testing.T) {
	m := NewInboxModel(testApp())
	m.loading = false
	m.items = []protocol.InboxItem{
		{SenderName: "alice", DeliveredAt: time.Now(), ReadAt: nil},
	}
	view := m.View()
	if !strings.Contains(view, "new") {
		t.Error("unread letter should show 'new' badge")
	}
}

// --- AddressBook Tests ---

func TestAddressBook_CursorNavigation(t *testing.T) {
	m := NewAddressBookModel(testApp())
	m.contacts = []protocol.ContactItem{
		{Username: "alice", HomeCity: "Boston"},
		{Username: "bob", HomeCity: "Denver"},
	}
	m.loading = false

	updated, _ := m.Update(keyMsg("j"))
	ab := updated.(AddressBookModel)
	if ab.cursor != 1 {
		t.Errorf("cursor should be 1, got %d", ab.cursor)
	}

	// Up at top stays at 0
	updated, _ = ab.Update(keyMsg("k"))
	ab = updated.(AddressBookModel)
	updated, _ = ab.Update(keyMsg("k"))
	ab = updated.(AddressBookModel)
	if ab.cursor != 0 {
		t.Errorf("cursor should stay at 0, got %d", ab.cursor)
	}
}

func TestAddressBook_AddContactNavigation(t *testing.T) {
	m := NewAddressBookModel(testApp())
	_, cmd := m.Update(keyMsg("a"))
	if cmd == nil {
		t.Fatal("expected command")
	}
	msg := cmd()
	sm, ok := msg.(switchScreenMsg)
	if !ok {
		t.Fatalf("expected switchScreenMsg, got %T", msg)
	}
	if sm.screen != ScreenAddContact {
		t.Errorf("expected AddContact screen, got %d", sm.screen)
	}
}

// --- AddContact Tests ---

func TestAddContact_InvalidAddress(t *testing.T) {
	m := NewAddContactModel(testApp())
	m.input.SetValue("invalidformat")
	updated, _ := m.Update(keyMsg("enter"))
	ac := updated.(AddContactModel)
	if ac.err == "" {
		t.Error("should show error for invalid address format")
	}
}

func TestAddContact_ValidAddressFormat(t *testing.T) {
	m := NewAddContactModel(testApp())
	m.input.SetValue("alice#1234")
	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("expected a command for valid address")
	}
	// The cmd is async (calls Network.AddContact), so we just verify it was dispatched
}

func TestAddContact_PostSuccessNavigation(t *testing.T) {
	m := NewAddContactModel(testApp())
	m.added = &protocol.ContactItem{Username: "alice", HomeCity: "Boston"}

	// Enter should go back to address book
	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("expected command after enter on success")
	}
	msg := cmd()
	sm, ok := msg.(switchScreenMsg)
	if !ok {
		t.Fatalf("expected switchScreenMsg, got %T", msg)
	}
	if sm.screen != ScreenAddressBook {
		t.Errorf("expected AddressBook, got %d", sm.screen)
	}
}

func TestAddContact_AddAnotherAfterSuccess(t *testing.T) {
	m := NewAddContactModel(testApp())
	m.added = &protocol.ContactItem{Username: "alice"}

	// 'n' should create a fresh AddContactModel
	updated, _ := m.Update(keyMsg("n"))
	ac := updated.(AddContactModel)
	if ac.added != nil {
		t.Error("new add contact should have no added contact")
	}
	if ac.input.Value() != "" {
		t.Error("input should be empty in fresh model")
	}
}

// --- Compose Tests ---

func TestCompose_RecipientSelection(t *testing.T) {
	m := NewComposeModel(testApp())
	contacts := []protocol.ContactItem{
		{UserID: uuid.New(), Username: "alice", HomeCity: "Boston"},
		{UserID: uuid.New(), Username: "bob", HomeCity: "Denver"},
	}

	// Simulate contacts loaded
	updated, _ := m.Update(contactsLoadedMsg{contacts: contacts})
	cm := updated.(ComposeModel)
	if len(cm.contacts) != 2 {
		t.Fatalf("expected 2 contacts, got %d", len(cm.contacts))
	}
	if len(cm.filteredIdx) != 2 {
		t.Fatalf("expected 2 filtered, got %d", len(cm.filteredIdx))
	}

	// Select first contact (enter)
	updated, _ = cm.Update(keyMsg("enter"))
	cm = updated.(ComposeModel)
	if cm.step != 1 {
		t.Errorf("expected step 1 (body), got %d", cm.step)
	}
	if cm.recipientName != "alice" {
		t.Errorf("expected recipient alice, got %s", cm.recipientName)
	}
}

func TestCompose_RecipientArrowKeys(t *testing.T) {
	m := NewComposeModel(testApp())
	contacts := []protocol.ContactItem{
		{UserID: uuid.New(), Username: "alice"},
		{UserID: uuid.New(), Username: "bob"},
	}
	updated, _ := m.Update(contactsLoadedMsg{contacts: contacts})
	cm := updated.(ComposeModel)

	// Move down
	updated, _ = cm.Update(keyMsg("down"))
	cm = updated.(ComposeModel)
	if cm.recipientSel != 1 {
		t.Errorf("expected selection 1, got %d", cm.recipientSel)
	}

	// Select bob
	updated, _ = cm.Update(keyMsg("enter"))
	cm = updated.(ComposeModel)
	if cm.recipientName != "bob" {
		t.Errorf("expected bob, got %s", cm.recipientName)
	}
}

func TestCompose_PrefilledRecipient(t *testing.T) {
	recipientID := uuid.New()
	m := NewComposeModelTo(testApp(), recipientID, "jake", uuid.Nil, "")
	if m.step != 1 {
		t.Errorf("pre-filled compose should start at step 1, got %d", m.step)
	}
	if m.recipientName != "jake" {
		t.Errorf("expected jake, got %s", m.recipientName)
	}
	if m.recipientID != recipientID {
		t.Error("recipient ID mismatch")
	}
}

func TestCompose_ShippingNavigation(t *testing.T) {
	m := NewComposeModel(testApp())
	m.step = 3

	// Default is first class (idx=0)
	if m.shippingIdx != 0 {
		t.Errorf("default shipping should be 0 (first class), got %d", m.shippingIdx)
	}

	// Press down → priority (1)
	updated, _ := m.Update(keyMsg("down"))
	cm := updated.(ComposeModel)
	if cm.shippingIdx != 1 {
		t.Errorf("expected 1, got %d", cm.shippingIdx)
	}

	// Press down → express (2)
	updated, _ = cm.Update(keyMsg("down"))
	cm = updated.(ComposeModel)
	if cm.shippingIdx != 2 {
		t.Errorf("expected 2, got %d", cm.shippingIdx)
	}

	// Down again shouldn't exceed 2
	updated, _ = cm.Update(keyMsg("down"))
	cm = updated.(ComposeModel)
	if cm.shippingIdx != 2 {
		t.Errorf("should stay at 2, got %d", cm.shippingIdx)
	}
}

func TestCompose_ShippingNumberKeys(t *testing.T) {
	m := NewComposeModel(testApp())
	m.step = 3

	updated, _ := m.Update(keyMsg("1"))
	cm := updated.(ComposeModel)
	if cm.shippingIdx != 0 {
		t.Errorf("pressing 1 should select express (0), got %d", cm.shippingIdx)
	}

	updated, _ = cm.Update(keyMsg("3"))
	cm = updated.(ComposeModel)
	if cm.shippingIdx != 2 {
		t.Errorf("pressing 3 should select express (2), got %d", cm.shippingIdx)
	}
}

func TestCompose_ShippingEscGoesBackToStamp(t *testing.T) {
	m := NewComposeModel(testApp())
	m.step = 3

	updated, _ := m.Update(keyMsg("esc"))
	cm := updated.(ComposeModel)
	if cm.step != 2 {
		t.Errorf("esc from shipping should go to stamp (step 2), got %d", cm.step)
	}
}

func TestCompose_ShippingOptions(t *testing.T) {
	m := NewComposeModel(testApp())
	opts := m.shippingOptions()
	if len(opts) != 3 {
		t.Fatalf("expected 3 shipping options, got %d", len(opts))
	}
	names := []string{"First Class", "Priority", "Express"}
	for i, opt := range opts {
		if opt.name != names[i] {
			t.Errorf("option %d: expected %s, got %s", i, names[i], opt.name)
		}
	}
}

// --- InTransit Tests ---

func TestInTransit_CursorAndTracking(t *testing.T) {
	m := NewInTransitModel(testApp())
	msgID := uuid.New()
	m.items = []protocol.InTransitItem{
		{MessageID: msgID, SenderName: "alice", OriginCity: "Boston"},
	}
	m.loading = false

	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("expected track command")
	}
	msg := cmd()
	tl, ok := msg.(trackLetterMsg)
	if !ok {
		t.Fatalf("expected trackLetterMsg, got %T", msg)
	}
	if tl.msgID != msgID {
		t.Error("tracking wrong message")
	}
}

// --- Sent Tests ---

func TestSent_EmptyView(t *testing.T) {
	m := NewSentModel(testApp())
	m.loading = false
	view := m.View()
	if !strings.Contains(view, "no letters yet") {
		t.Error("empty sent should show 'no letters yet'")
	}
}

func TestSent_TrackKey(t *testing.T) {
	msgID := uuid.New()
	m := NewSentModel(testApp())
	m.items = []protocol.SentItem{
		{MessageID: msgID, RecipientName: "alice", Status: "in_transit", SentAt: time.Now()},
	}
	m.loading = false

	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("expected track command")
	}
	msg := cmd()
	tl, ok := msg.(trackLetterMsg)
	if !ok {
		t.Fatalf("expected trackLetterMsg, got %T", msg)
	}
	if tl.msgID != msgID {
		t.Error("tracking wrong message")
	}
}

// --- ReadLetter Tests ---

func TestReadLetter_ReplyKey(t *testing.T) {
	senderID := uuid.New()
	item := protocol.InboxItem{
		SenderID:    senderID,
		SenderName:  "alice",
		DeliveredAt: time.Now(),
		SentAt:      time.Now(),
	}
	m := NewReadLetterModel(testApp(), item, "test body")

	_, cmd := m.Update(keyMsg("r"))
	if cmd == nil {
		t.Fatal("expected reply command")
	}
	msg := cmd()
	ct, ok := msg.(composeToMsg)
	if !ok {
		t.Fatalf("expected composeToMsg, got %T", msg)
	}
	if ct.recipientID != senderID {
		t.Error("reply should target the sender")
	}
}

func TestReadLetter_DecryptedView(t *testing.T) {
	item := protocol.InboxItem{
		SenderName:  "alice",
		DeliveredAt: time.Now(),
		SentAt:      time.Now(),
	}
	m := NewReadLetterModel(testApp(), item, "Hello from Boston!")
	view := m.View()
	if !strings.Contains(view, "Hello from Boston!") {
		t.Error("view should show decrypted body")
	}
}

// --- Error handling ---

func TestInbox_ErrorMsg(t *testing.T) {
	m := NewInboxModel(testApp())
	updated, _ := m.Update(errMsg{err: fmt.Errorf("connection lost")})
	im := updated.(InboxModel)
	if im.err != "connection lost" {
		t.Errorf("expected error 'connection lost', got %q", im.err)
	}
	if im.loading {
		t.Error("should not be loading after error")
	}
}

// --- TUI Root Tests ---

func TestTUI_ScreenSwitching(t *testing.T) {
	app := testApp()
	tui := NewTUI(app)

	// Should start at home since UserID is set and no PIN file exists
	if tui.screen != ScreenHome {
		t.Errorf("expected ScreenHome, got %d", tui.screen)
	}
}

func TestTUI_UnregisteredStartsAtRegister(t *testing.T) {
	app := testApp()
	app.UserID = uuid.UUID{} // zero UUID = not registered
	tui := NewTUI(app)

	if tui.screen != ScreenRegister {
		t.Errorf("expected ScreenRegister, got %d", tui.screen)
	}
}

func TestTUI_SwitchScreenMsg(t *testing.T) {
	app := testApp()
	tui := NewTUI(app)

	// Switch to compose
	updated, _ := tui.Update(switchScreenMsg{screen: ScreenCompose})
	root := updated.(TUI)
	if root.screen != ScreenCompose {
		t.Errorf("expected ScreenCompose, got %d", root.screen)
	}
}

func TestTUI_ComposeToMsg(t *testing.T) {
	app := testApp()
	tui := NewTUI(app)

	recipientID := uuid.New()
	updated, _ := tui.Update(composeToMsg{recipientID: recipientID, recipientName: "alice"})
	root := updated.(TUI)
	if root.screen != ScreenCompose {
		t.Errorf("expected ScreenCompose, got %d", root.screen)
	}
}

// --- Reply Reference Toggle Tests ---

func TestCompose_CtrlRTogglesOriginal(t *testing.T) {
	app := testApp()
	msgID := uuid.New()
	app.DecryptedBodies[msgID] = "Hello from the original letter"

	m := NewComposeModelTo(app, uuid.New(), "alice", msgID, "alice")
	if m.showingOriginal {
		t.Fatal("should not start in original view")
	}

	// Press ctrl+r to show original
	updated, _ := m.Update(ctrlKeyMsg(tea.KeyCtrlR))
	cm := updated.(ComposeModel)
	if !cm.showingOriginal {
		t.Error("ctrl+r should toggle to original view")
	}

	// Press ctrl+r again to go back
	updated, _ = cm.Update(ctrlKeyMsg(tea.KeyCtrlR))
	cm = updated.(ComposeModel)
	if cm.showingOriginal {
		t.Error("ctrl+r should toggle back to compose")
	}
}

func TestCompose_CtrlRNoOpOnFreshCompose(t *testing.T) {
	m := NewComposeModelTo(testApp(), uuid.New(), "alice", uuid.Nil, "")
	m.step = 1

	updated, _ := m.Update(ctrlKeyMsg(tea.KeyCtrlR))
	cm := updated.(ComposeModel)
	if cm.showingOriginal {
		t.Error("ctrl+r should be a no-op on fresh compose (no original)")
	}
}

func TestCompose_TextareaPreservedAcrossToggle(t *testing.T) {
	app := testApp()
	msgID := uuid.New()
	app.DecryptedBodies[msgID] = "Original content"

	m := NewComposeModelTo(app, uuid.New(), "alice", msgID, "alice")

	// Type some text by setting the textarea value directly
	m.bodyArea.SetValue("My reply text here")

	// Toggle to original
	updated, _ := m.Update(ctrlKeyMsg(tea.KeyCtrlR))
	cm := updated.(ComposeModel)
	if !cm.showingOriginal {
		t.Fatal("should be showing original")
	}

	// Toggle back
	updated, _ = cm.Update(ctrlKeyMsg(tea.KeyCtrlR))
	cm = updated.(ComposeModel)
	if cm.showingOriginal {
		t.Fatal("should be back to compose")
	}

	if cm.bodyArea.Value() != "My reply text here" {
		t.Errorf("textarea content should be preserved, got %q", cm.bodyArea.Value())
	}
}

func TestCompose_ReplyFromInboxDecryptsBody(t *testing.T) {
	app := testApp()
	senderID := uuid.New()
	msgID := uuid.New()

	m := NewInboxModel(app)
	m.items = []protocol.InboxItem{
		{
			MessageID:    msgID,
			SenderID:     senderID,
			SenderName:   "alice",
			DeliveredAt:  time.Now(),
			SenderPubKey: nil, // will fail decrypt gracefully
		},
	}
	m.loading = false

	_, cmd := m.Update(keyMsg("r"))
	if cmd == nil {
		t.Fatal("expected compose command")
	}
	msg := cmd()
	ct, ok := msg.(composeToMsg)
	if !ok {
		t.Fatalf("expected composeToMsg, got %T", msg)
	}
	if ct.originalMsgID != msgID {
		t.Error("reply should include original message ID")
	}
	if ct.originalSender != "alice" {
		t.Errorf("expected original sender alice, got %s", ct.originalSender)
	}
	// Body should be cached in DecryptedBodies (even if decrypt failed)
	if _, ok := app.DecryptedBodies[msgID]; !ok {
		t.Error("DecryptedBodies should have entry for the message")
	}
}

func TestCompose_OriginalViewContent(t *testing.T) {
	app := testApp()
	msgID := uuid.New()
	app.DecryptedBodies[msgID] = "The original letter body"

	m := NewComposeModelTo(app, uuid.New(), "bob", msgID, "bob")

	// Toggle to original
	updated, _ := m.Update(ctrlKeyMsg(tea.KeyCtrlR))
	cm := updated.(ComposeModel)

	view := cm.View()
	if !strings.Contains(view, "ORIGINAL LETTER") {
		t.Error("original view should show ORIGINAL LETTER title")
	}
	if !strings.Contains(view, "bob") {
		t.Error("original view should show sender name")
	}
	if !strings.Contains(view, "back to reply") {
		t.Error("original view should show back to reply hint")
	}
}

// --- Draft Tests ---

func TestDraft_SaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PENPAL_HOME", tmp)

	recipientID := uuid.New()
	original := Draft{
		RecipientID:    recipientID,
		RecipientName:  "alice",
		Body:           "Dear Alice, how are you?",
		OriginalMsgID:  uuid.New(),
		OriginalSender: "bob",
	}
	if err := SaveDraft(original); err != nil {
		t.Fatalf("SaveDraft failed: %v", err)
	}

	loaded, err := LoadDraft(recipientID)
	if err != nil {
		t.Fatalf("LoadDraft failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected draft, got nil")
	}
	if loaded.Body != original.Body {
		t.Errorf("body mismatch: got %q", loaded.Body)
	}
	if loaded.RecipientName != "alice" {
		t.Errorf("recipient name mismatch: got %q", loaded.RecipientName)
	}
	if loaded.OriginalMsgID != original.OriginalMsgID {
		t.Error("original msg ID mismatch")
	}
}

func TestDraft_LoadMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PENPAL_HOME", tmp)

	d, err := LoadDraft(uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != nil {
		t.Error("expected nil for missing draft")
	}
}

func TestDraft_DeleteOnSend(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PENPAL_HOME", tmp)

	recipientID := uuid.New()
	if err := SaveDraft(Draft{
		RecipientID:   recipientID,
		RecipientName: "alice",
		Body:          "test body",
	}); err != nil {
		t.Fatalf("SaveDraft failed: %v", err)
	}

	// Verify it exists
	d, _ := LoadDraft(recipientID)
	if d == nil {
		t.Fatal("draft should exist after save")
	}

	// Delete
	if err := DeleteDraft(recipientID); err != nil {
		t.Fatalf("DeleteDraft failed: %v", err)
	}

	// Verify it's gone
	d, _ = LoadDraft(recipientID)
	if d != nil {
		t.Error("draft should be nil after delete")
	}
}

func TestDraft_EmptyBodyDeletes(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PENPAL_HOME", tmp)

	recipientID := uuid.New()
	// Save a draft first
	if err := SaveDraft(Draft{
		RecipientID:   recipientID,
		RecipientName: "alice",
		Body:          "some text",
	}); err != nil {
		t.Fatalf("SaveDraft failed: %v", err)
	}

	// saveDraft with empty body should delete
	m := NewComposeModel(testApp())
	m.recipientID = recipientID
	m.recipientName = "alice"
	m.bodyArea.SetValue("   ") // whitespace-only
	m.saveDraft()

	d, _ := LoadDraft(recipientID)
	if d != nil {
		t.Error("draft should be deleted when body is empty/whitespace")
	}
}

func TestCompose_DraftRestoredNotice(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PENPAL_HOME", tmp)

	recipientID := uuid.New()
	if err := SaveDraft(Draft{
		RecipientID:   recipientID,
		RecipientName: "alice",
		Body:          "saved draft text",
	}); err != nil {
		t.Fatalf("SaveDraft failed: %v", err)
	}

	m := NewComposeModelTo(testApp(), recipientID, "alice", uuid.Nil, "")
	if !m.draftRestored {
		t.Error("draftRestored should be true after loading a draft")
	}
	view := m.View()
	if !strings.Contains(view, "draft restored") {
		t.Error("view should contain 'draft restored' notice")
	}
	if m.bodyArea.Value() != "saved draft text" {
		t.Errorf("body should be pre-filled with draft, got %q", m.bodyArea.Value())
	}
}

func TestCompose_ViewBodyShowsCtrlRHint(t *testing.T) {
	app := testApp()
	msgID := uuid.New()
	app.DecryptedBodies[msgID] = "body"

	m := NewComposeModelTo(app, uuid.New(), "alice", msgID, "alice")
	view := m.View()
	if !strings.Contains(view, "ctrl+r") {
		t.Error("compose view should show ctrl+r hint when replying")
	}

	// Fresh compose should NOT show ctrl+r hint
	m2 := NewComposeModelTo(testApp(), uuid.New(), "bob", uuid.Nil, "")
	view2 := m2.View()
	if strings.Contains(view2, "ctrl+r") {
		t.Error("fresh compose should not show ctrl+r hint")
	}
}
