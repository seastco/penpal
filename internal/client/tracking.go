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
	"github.com/seastco/penpal/internal/models"
	"github.com/seastco/penpal/internal/protocol"
)

// TrackingModel displays the relay log for a letter.
type TrackingModel struct {
	app      *AppState
	msgID    uuid.UUID
	label    string // "-> jake" or "from jake"
	origin   Screen // screen to return to on back
	tracking *protocol.TrackingResponse
	loading  bool
	err      string
}

func NewTrackingModel(app *AppState, msgID uuid.UUID, label string, origin Screen) TrackingModel {
	return TrackingModel{
		app:     app,
		msgID:   msgID,
		label:   label,
		origin:  origin,
		loading: true,
	}
}

type trackingLoadedMsg struct {
	tracking *protocol.TrackingResponse
}

func (m TrackingModel) Init() tea.Cmd {
	return m.loadTracking()
}

func (m TrackingModel) loadTracking() tea.Cmd {
	return func() tea.Msg {
		resp, err := m.app.Network.Send(context.Background(), protocol.MsgGetTracking, protocol.GetTrackingRequest{
			MessageID: m.msgID,
		})
		if err != nil {
			return errMsg{err: err}
		}
		data, _ := json.Marshal(resp.Payload)
		var tracking protocol.TrackingResponse
		if err := json.Unmarshal(data, &tracking); err != nil {
			return errMsg{err: fmt.Errorf("parsing tracking: %w", err)}
		}
		return trackingLoadedMsg{tracking: &tracking}
	}
}

func (m TrackingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "b", "esc":
			return m, func() tea.Msg { return switchScreenMsg{screen: m.origin} }
		}
	case trackingLoadedMsg:
		m.tracking = msg.tracking
		m.loading = false
	case errMsg:
		m.err = msg.err.Error()
		m.loading = false
	}
	return m, nil
}

func (m TrackingModel) View() tea.View {
	title := titleStyle.Render(fmt.Sprintf("TRACKING %s", m.label))
	header := title + "\n" + divider(contentWidth()) + "\n"

	if m.loading {
		return tea.NewView(emptyScreenView(header, "", "[b] back"))
	}
	if m.err != "" {
		return tea.NewView(emptyScreenView(header, "\n"+errorStyle.Render(m.err), "[b] back"))
	}
	if m.tracking == nil {
		return tea.NewView(emptyScreenView(header, "", "[b] back"))
	}

	content := header
	now := time.Now()
	total := len(m.tracking.Route)

	// Delivery status
	if total > 0 {
		lastHop := m.tracking.Route[total-1]
		if time.Until(lastHop.ETA) <= 0 {
			content += "\n" + successStyle.Render("Delivered!") + "\n"
		}
	}

	content += "\n"
	for _, hop := range m.tracking.Route {
		var timeStr string
		node := mutedStyle.Render("○")
		if now.After(hop.ETA) && isCurrentHop(m.tracking.Route, hop, now) {
			node = selectedStyle.Render("◉")
			timeStr = hop.ETA.Local().Format("01/02  15:04")
		} else if now.After(hop.ETA) {
			node = successStyle.Render("●")
			timeStr = hop.ETA.Local().Format("01/02  15:04")
		} else {
			timeStr = hop.ETA.Local().Format("01/02 ~15:04")
		}
		content += fmt.Sprintf("%s %s  %s\n", node, mutedStyle.Render(timeStr), hop.City)
	}

	content += fmt.Sprintf("\n%s\n",
		mutedStyle.Render(fmt.Sprintf("%s · %.0f mi",
			m.tracking.ShippingTier, m.tracking.Distance)))

	content += "\n" + helpStyle.Render("[b] back")
	return tea.NewView(screenBox().Render(content))
}

func isCurrentHop(route []models.RouteHop, hop models.RouteHop, now time.Time) bool {
	for i, h := range route {
		if h.ETA == hop.ETA {
			if i+1 < len(route) && now.Before(route[i+1].ETA) {
				return true
			}
		}
	}
	return false
}

// InTransitModel shows all letters currently in transit (incoming and outgoing).
type InTransitModel struct {
	app      *AppState
	items    []protocol.InTransitItem
	cursor   int
	viewport viewport.Model
	loading  bool
	err      string
}

func NewInTransitModel(app *AppState) InTransitModel {
	vp := viewport.New(viewport.WithWidth(contentWidth()), viewport.WithHeight(viewportHeight()))
	vp.KeyMap = viewport.KeyMap{}
	m := InTransitModel{app: app, loading: true, viewport: vp}
	return m.syncViewport()
}

type inTransitLoadedMsg struct {
	items []protocol.InTransitItem
}

func (m InTransitModel) Init() tea.Cmd {
	return func() tea.Msg {
		items, err := m.app.Network.GetInTransit(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		return inTransitLoadedMsg{items: items}
	}
}

func (m InTransitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.items) > 0 {
				item := m.items[m.cursor]
				label := "from " + item.PeerName
				if item.Direction == "outgoing" {
					label = "to " + item.PeerName
				}
				return m, func() tea.Msg {
					return trackLetterMsg{msgID: item.MessageID, label: label, origin: ScreenInTransit}
				}
			}
		case "b", "esc":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenHome} }
		}
	case tea.WindowSizeMsg:
		m.viewport.SetWidth(contentWidth())
	case inTransitLoadedMsg:
		m.items = msg.items
		m.loading = false
	case errMsg:
		m.err = msg.err.Error()
		m.loading = false
	}
	m = m.syncViewport()
	return m, nil
}

const linesPerTransitItem = 5

func (m InTransitModel) syncViewport() InTransitModel {
	bh := adaptiveBoxHeight(len(m.items)*linesPerTransitItem, 6)
	m.viewport.SetHeight(bh - 6)

	var content string
	if m.err != "" {
		content = "\n" + errorStyle.Render(m.err)
	} else if len(m.items) == 0 {
		content = "\n" + mutedStyle.Render("no letters in transit")
	} else {
		now := time.Now()
		var b strings.Builder
		for i, item := range m.items {
			prefix := "  "
			if i == m.cursor {
				prefix = "> "
			}

			dirLabel := "from"
			if item.Direction == "outgoing" {
				dirLabel = "  to"
			}

			currentHop := 0
			totalHops := len(item.Route)
			for j := len(item.Route) - 1; j >= 0; j-- {
				if now.After(item.Route[j].ETA) {
					currentHop = j
					break
				}
			}
			currentCity := "en route"
			if currentHop < len(item.Route) {
				currentCity = item.Route[currentHop].City
			}

			est := item.ReleaseAt.Format("Jan 2")

			line := fmt.Sprintf("%s%s: %s", prefix, dirLabel, item.PeerName)
			if i == m.cursor {
				b.WriteString(selectedStyle.Render(line) + "\n")
			} else {
				b.WriteString(line + "\n")
			}
			b.WriteString(fmt.Sprintf("    %s -> %s\n",
				mutedStyle.Render(item.OriginCity), mutedStyle.Render(item.DestCity)))
			b.WriteString(fmt.Sprintf("    %s  %s (hop %d/%d)\n",
				mutedStyle.Render(item.ShippingTier),
				mutedStyle.Render(currentCity), currentHop+1, totalHops))
			b.WriteString(fmt.Sprintf("    est. arrival: ~%s\n", mutedStyle.Render(est)))
			b.WriteString("\n")
		}
		content = b.String()
	}

	yOffset := m.viewport.YOffset()
	m.viewport.SetContent(content)
	if len(m.items) > 0 {
		m.viewport.SetYOffset(yOffset)
		cursorLine := m.cursor * linesPerTransitItem
		if cursorLine < m.viewport.YOffset() {
			m.viewport.SetYOffset(cursorLine)
		} else if cursorLine+linesPerTransitItem > m.viewport.YOffset()+m.viewport.Height() {
			m.viewport.SetYOffset(cursorLine + linesPerTransitItem - m.viewport.Height())
		}
	}
	return m
}

func (m InTransitModel) View() tea.View {
	title := titleStyle.Render("IN TRANSIT")
	header := title + "\n" + divider(contentWidth()) + "\n"
	if m.loading {
		return tea.NewView(emptyScreenView(header, "", "[b] back"))
	}
	if len(m.items) == 0 {
		body := "\n" + mutedStyle.Render("no letters in transit")
		if m.err != "" {
			body = "\n" + errorStyle.Render(m.err)
		}
		return tea.NewView(emptyScreenView(header, body, "[b] back"))
	}
	m = m.syncViewport()
	bh := adaptiveBoxHeight(len(m.items)*linesPerTransitItem, 6)
	footer := "\n\n" + helpStyle.Render("[enter] view  [b] back")
	return tea.NewView(screenBox().Height(bh).Render(header + m.viewport.View() + footer))
}

type trackLetterMsg struct {
	msgID  uuid.UUID
	label  string
	origin Screen
}

// SentModel shows sent letters.
type SentModel struct {
	app           *AppState
	items         []protocol.SentItem
	cursor        int
	viewport      viewport.Model
	loading       bool
	loadingMore   bool
	hasMore       bool
	confirmDelete bool
	err           string
}

func NewSentModel(app *AppState) SentModel {
	vp := viewport.New(viewport.WithWidth(contentWidth()), viewport.WithHeight(viewportHeight()))
	vp.KeyMap = viewport.KeyMap{}
	m := SentModel{app: app, loading: true, viewport: vp}
	return m.syncViewport()
}

type sentLoadedMsg struct {
	items   []protocol.SentItem
	hasMore bool
	append  bool // true = subsequent page (append to existing), false = first page (replace)
}

func (m SentModel) Init() tea.Cmd {
	return m.fetchSent(nil, false)
}

func (m SentModel) fetchSent(before *time.Time, append bool) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.app.Network.GetSent(context.Background(), before)
		if err != nil {
			return errMsg{err: err}
		}
		return sentLoadedMsg{items: resp.Letters, hasMore: resp.HasMore, append: append}
	}
}

func (m SentModel) maybePrefetch() tea.Cmd {
	if m.hasMore && !m.loadingMore && m.cursor >= len(m.items)-50 && len(m.items) > 0 {
		last := m.items[len(m.items)-1]
		cursor := last.SentAt
		m.loadingMore = true
		return m.fetchSent(&cursor, true)
	}
	return nil
}

func (m SentModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
					resp, err := m.app.Network.GetSent(ctx, nil)
					if err != nil {
						return errMsg{err: err}
					}
					return sentLoadedMsg{items: resp.Letters, hasMore: resp.HasMore}
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
				return m, func() tea.Msg {
					return trackLetterMsg{msgID: item.MessageID, label: "-> " + item.RecipientName, origin: ScreenSent}
				}
			}
		case "b", "esc":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenHome} }
		}
	case tea.WindowSizeMsg:
		m.viewport.SetWidth(contentWidth())
	case sentLoadedMsg:
		if msg.append {
			m.items = append(m.items, msg.items...)
			m.loadingMore = false
		} else {
			m.items = msg.items
			m.loading = false
		}
		if m.cursor >= len(m.items) && m.cursor > 0 {
			m.cursor = len(m.items) - 1
		}
		m.hasMore = msg.hasMore
	case errMsg:
		m.err = msg.err.Error()
		m.loading = false
		m.loadingMore = false
	}
	m = m.syncViewport()
	return m, nil
}

func (m SentModel) syncViewport() SentModel {
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
				prefix = "> "
			}
			date := item.SentAt.Format("Jan 2")
			status := mutedStyle.Render(item.Status)
			if item.Status == "delivered" {
				status = successStyle.Render("delivered")
			}
			line := fmt.Sprintf("%-14s %s  %s", item.RecipientName, date, status)
			if i == m.cursor {
				b.WriteString(selectedStyle.Render(prefix+line) + "\n")
			} else {
				b.WriteString(prefix + line + "\n")
			}
		}
		content = b.String()
	}

	yOffset := m.viewport.YOffset()
	m.viewport.SetContent(content)
	if len(m.items) > 0 {
		m.viewport.SetYOffset(yOffset)
		if m.cursor < m.viewport.YOffset() {
			m.viewport.SetYOffset(m.cursor)
		} else if m.cursor >= m.viewport.YOffset()+m.viewport.Height() {
			m.viewport.SetYOffset(m.cursor - m.viewport.Height() + 1)
		}
	}
	return m
}

func (m SentModel) View() tea.View {
	title := titleStyle.Render("SENT")
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
		name := m.items[m.cursor].RecipientName
		footer = "\n\n" + errorStyle.Render(fmt.Sprintf("delete letter to %s? [y] yes  [n] no", name))
	} else {
		footer = "\n\n" + helpStyle.Render("[enter] view  [d] delete  [b] back")
	}
	return tea.NewView(screenBox().Height(bh).Render(header + m.viewport.View() + footer))
}
