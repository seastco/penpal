package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stove/penpal/internal/models"
	"github.com/stove/penpal/internal/protocol"
)

// --- Emoji mappings ---

// All emojis must be natively width-2 (Emoji_Presentation=Yes) without VS16 (U+FE0F).
// VS16 emojis render at inconsistent widths across terminals (especially iTerm2).
var stateEmoji = map[string]string{
	"AK": "🐻", "AL": "🚀", "AZ": "🌵", "AR": "💎", "CA": "🌅", "CO": "🗻",
	"CT": "⚓", "DE": "🐔", "FL": "🌴", "GA": "🍑", "HI": "🌺", "ID": "🥔",
	"IL": "🌆", "IN": "🏁", "IA": "🚜", "KS": "🌾", "KY": "🐎", "LA": "🎺",
	"ME": "🦞", "MD": "🦀", "MA": "🎓", "MI": "🚗", "MN": "⛄", "MS": "🛶",
	"MO": "🌉", "MT": "🦬", "NE": "🌽", "NV": "🎰", "NH": "🌄", "NJ": "🎡",
	"NM": "🎨", "NY": "🗽", "NC": "🛫", "ND": "🦅", "OH": "🏈", "OK": "⛽",
	"OR": "🦫", "PA": "🔔", "RI": "⛵", "SC": "🌙", "SD": "🗿", "TN": "🎸",
	"TX": "🤠", "UT": "🪨", "VT": "🍁", "VA": "🦌", "WA": "🌲", "WV": "🔨",
	"WI": "🧀", "WY": "🐺",
}

var countryEmoji = map[string]string{
	"ES": "🇪🇸",
}

var countryStampNames = map[string]string{
	"ES": "Spain",
}

var stampEmoji = map[string]string{
	"common:flag":        "🇺🇸",
	"common:heart":       "💕",
	"common:star":        "🌟",
	"common:quill":       "🪶",
	"common:blossom":     "🌸",
	"common:sunflower":   "🌻",
	"common:butterfly":   "🦋",
	"common:wave":        "🌊",
	"common:moon":        "🌙",
	"common:bird":        "🐦",
	"common:rainbow":     "🌈",
	"common:clover":      "🍀",
	"rare:cross_country": "🌎",
	"rare:explorer":      "🧭",
	"rare:penpal":        "💌",
	"rare:faithful":      "📬",
	"rare:collector":     "🏆",
}

// stateNames maps 2-letter codes to full state names.
var stateNames = map[string]string{
	"AK": "Alaska", "AL": "Alabama", "AR": "Arkansas", "AZ": "Arizona",
	"CA": "California", "CO": "Colorado", "CT": "Connecticut",
	"DE": "Delaware", "FL": "Florida", "GA": "Georgia",
	"HI": "Hawaii", "IA": "Iowa", "ID": "Idaho",
	"IL": "Illinois", "IN": "Indiana",
	"KS": "Kansas", "KY": "Kentucky",
	"LA": "Louisiana", "MA": "Massachusetts", "MD": "Maryland",
	"ME": "Maine", "MI": "Michigan", "MN": "Minnesota",
	"MO": "Missouri", "MS": "Mississippi", "MT": "Montana",
	"NC": "North Carolina", "ND": "North Dakota", "NE": "Nebraska",
	"NH": "New Hampshire", "NJ": "New Jersey", "NM": "New Mexico",
	"NV": "Nevada", "NY": "New York",
	"OH": "Ohio", "OK": "Oklahoma", "OR": "Oregon",
	"PA": "Pennsylvania", "RI": "Rhode Island",
	"SC": "South Carolina", "SD": "South Dakota",
	"TN": "Tennessee", "TX": "Texas", "UT": "Utah",
	"VA": "Virginia", "VT": "Vermont", "WA": "Washington",
	"WI": "Wisconsin", "WV": "West Virginia", "WY": "Wyoming",
}

// Ordered list of all 50 state codes (alphabetical).
var allStateCodes = []string{
	"AK", "AL", "AR", "AZ", "CA", "CO", "CT", "DE",
	"FL", "GA", "HI", "IA", "ID", "IL", "IN", "KS",
	"KY", "LA", "MA", "MD", "ME", "MI", "MN", "MO",
	"MS", "MT", "NC", "ND", "NE", "NH", "NJ", "NM",
	"NV", "NY", "OH", "OK", "OR", "PA", "RI", "SC",
	"SD", "TN", "TX", "UT", "VA", "VT", "WA", "WI",
	"WV", "WY",
}

var commonSlots = []string{
	"common:flag", "common:heart", "common:star", "common:quill",
	"common:blossom", "common:sunflower", "common:butterfly", "common:wave",
	"common:moon", "common:bird", "common:rainbow", "common:clover",
}
var rareSlots = []string{
	"rare:cross_country", "rare:explorer", "rare:penpal", "rare:faithful", "rare:collector",
}

type stampSlot struct {
	stampType   string
	displayName string
	emoji       string
	rarity      models.StampRarity
	collected   bool
	count       int
	stamp       *models.Stamp // nil if uncollected
}

type stampCategory struct {
	name     string
	slots    []stampSlot
	startIdx int // index in allSlots
}

// StampsModel shows the user's stamp collection.
type StampsModel struct {
	app        *AppState
	stamps     []models.Stamp
	allSlots   []stampSlot
	categories []stampCategory
	cursor     int
	viewport   viewport.Model
	loading    bool
	err        string
	detailMode bool
}

func NewStampsModel(app *AppState) StampsModel {
	vp := viewport.New(contentWidth(), viewportHeight())
	vp.KeyMap = viewport.KeyMap{}
	m := StampsModel{app: app, loading: true, viewport: vp}
	return m
}

type stampsLoadedMsg struct {
	stamps []models.Stamp
}

func (m StampsModel) Init() tea.Cmd {
	return func() tea.Msg {
		stamps, err := m.app.Network.GetStampsAll(context.Background())
		if err != nil {
			return errMsg{err: err}
		}
		return stampsLoadedMsg{stamps: stamps}
	}
}

func (m StampsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.detailMode {
			switch msg.String() {
			case "b", "esc":
				m.detailMode = false
			}
			m = m.syncViewport()
			return m, nil
		}
		switch msg.String() {
		case "left", "h":
			if m.cursor > 0 {
				m.cursor--
			}
		case "right", "l":
			if m.cursor < len(m.allSlots)-1 {
				m.cursor++
			}
		case "up", "k":
			cat, catIdx := m.categoryOf(m.cursor)
			col := (m.cursor - cat.startIdx) % stampsPerRow
			if m.cursor-stampsPerRow >= cat.startIdx {
				m.cursor -= stampsPerRow
			} else if catIdx > 0 {
				prev := m.categories[catIdx-1]
				lastRowStart := prev.startIdx + (len(prev.slots)-1)/stampsPerRow*stampsPerRow
				target := lastRowStart + col
				maxInPrev := prev.startIdx + len(prev.slots) - 1
				if target > maxInPrev {
					target = maxInPrev
				}
				m.cursor = target
			}
		case "down", "j":
			cat, catIdx := m.categoryOf(m.cursor)
			col := (m.cursor - cat.startIdx) % stampsPerRow
			currentRow := (m.cursor - cat.startIdx) / stampsPerRow
			lastRow := (len(cat.slots) - 1) / stampsPerRow
			if currentRow < lastRow {
				// Next row exists in same category — clamp column
				nextRowStart := cat.startIdx + (currentRow+1)*stampsPerRow
				target := nextRowStart + col
				maxInCat := cat.startIdx + len(cat.slots) - 1
				if target > maxInCat {
					target = maxInCat
				}
				m.cursor = target
			} else if catIdx+1 < len(m.categories) {
				next := m.categories[catIdx+1]
				target := next.startIdx + col
				maxInNext := next.startIdx + len(next.slots) - 1
				if target > maxInNext {
					target = maxInNext
				}
				m.cursor = target
			}
		case "enter":
			if len(m.allSlots) > 0 && m.allSlots[m.cursor].collected {
				m.detailMode = true
			}
		case "b", "esc":
			return m, func() tea.Msg { return switchScreenMsg{screen: ScreenHome} }
		}
	case tea.WindowSizeMsg:
		m.viewport.Width = contentWidth()
		m.viewport.Height = viewportHeight()
	case stampsLoadedMsg:
		m.stamps = msg.stamps
		m.loading = false
		m.buildSlots()
	case errMsg:
		m.err = msg.err.Error()
		m.loading = false
	}
	m = m.syncViewport()
	return m, nil
}

func (m *StampsModel) buildSlots() {
	// Aggregate stamps by type: count occurrences, keep most recent
	type stampAgg struct {
		stamp *models.Stamp
		count int
	}
	agg := make(map[string]*stampAgg)
	for i := range m.stamps {
		s := &m.stamps[i]
		key := strings.ToLower(s.StampType)
		if a, ok := agg[key]; ok {
			a.count++
			if s.CreatedAt.After(a.stamp.CreatedAt) {
				a.stamp = s
			}
		} else {
			agg[key] = &stampAgg{stamp: s, count: 1}
		}
	}

	m.allSlots = nil
	m.categories = nil

	// COMMON
	var cmSlots []stampSlot
	for _, key := range commonSlots {
		s := stampSlot{
			stampType:   key,
			displayName: commonDisplayName(key),
			emoji:       stampEmoji[key],
		}
		if key == "common:flag" {
			s.emoji = flagEmojiForCity(m.app.HomeCity)
			s.displayName = flagDisplayName(m.app.HomeCity)
		}
		if a, ok := agg[key]; ok {
			s.collected = true
			s.rarity = a.stamp.Rarity
			s.stamp = a.stamp
			s.count = a.count
		}
		cmSlots = append(cmSlots, s)
	}
	cmStart := len(m.allSlots)
	m.allSlots = append(m.allSlots, cmSlots...)
	m.categories = append(m.categories, stampCategory{
		name:     fmt.Sprintf("COMMON (%d/%d)", countCollected(cmSlots), len(cmSlots)),
		slots:    cmSlots,
		startIdx: cmStart,
	})

	// STATES
	var stateSlotsList []stampSlot
	for _, code := range allStateCodes {
		key := "state:" + strings.ToLower(code)
		a, ok := agg[key]
		if !ok {
			continue
		}
		s := stampSlot{
			stampType:   key,
			displayName: code,
			emoji:       stateEmoji[code],
			collected:   true,
			rarity:      a.stamp.Rarity,
			stamp:       a.stamp,
			count:       a.count,
		}
		stateSlotsList = append(stateSlotsList, s)
	}
	if len(stateSlotsList) > 0 {
		stateStart := len(m.allSlots)
		m.allSlots = append(m.allSlots, stateSlotsList...)
		m.categories = append(m.categories, stampCategory{
			name:     fmt.Sprintf("STATES (%d/%d)", len(stateSlotsList), len(allStateCodes)),
			slots:    stateSlotsList,
			startIdx: stateStart,
		})
	}

	// COUNTRIES — show only collected country stamps
	var countrySlotsList []stampSlot
	for key, a := range agg {
		if !strings.HasPrefix(key, "country:") {
			continue
		}
		code := strings.ToUpper(strings.TrimPrefix(key, "country:"))
		emoji := countryEmoji[code]
		if emoji == "" {
			emoji = "🌍"
		}
		name := countryStampNames[code]
		if name == "" {
			name = code
		}
		countrySlotsList = append(countrySlotsList, stampSlot{
			stampType:   key,
			displayName: smartTruncate(name, stampCardInnerW),
			emoji:       emoji,
			collected:   true,
			rarity:      a.stamp.Rarity,
			stamp:       a.stamp,
			count:       a.count,
		})
	}
	if len(countrySlotsList) > 0 {
		countryStart := len(m.allSlots)
		m.allSlots = append(m.allSlots, countrySlotsList...)
		m.categories = append(m.categories, stampCategory{
			name:     fmt.Sprintf("COUNTRIES (%d)", len(countrySlotsList)),
			slots:    countrySlotsList,
			startIdx: countryStart,
		})
	}

	// RARE — only show collected + one mystery placeholder (after states/countries)
	var rrSlots []stampSlot
	for _, key := range rareSlots {
		a, ok := agg[key]
		if !ok {
			continue
		}
		rrSlots = append(rrSlots, stampSlot{
			stampType:   key,
			displayName: rareDisplayName(key),
			emoji:       stampEmoji[key],
			collected:   true,
			rarity:      a.stamp.Rarity,
			stamp:       a.stamp,
			count:       a.count,
		})
	}
	if len(rrSlots) > 0 {
		// User has rare stamps — show count
		rrStart := len(m.allSlots)
		m.allSlots = append(m.allSlots, rrSlots...)
		m.categories = append(m.categories, stampCategory{
			name:     fmt.Sprintf("RARE (%d)", len(rrSlots)),
			slots:    rrSlots,
			startIdx: rrStart,
		})
	} else {
		// No rare stamps yet — show single mystery placeholder
		rrSlots = []stampSlot{{
			stampType: "rare:unknown",
			emoji:     "?",
			collected: false,
		}}
		rrStart := len(m.allSlots)
		m.allSlots = append(m.allSlots, rrSlots...)
		m.categories = append(m.categories, stampCategory{
			name:     "RARE (?)",
			slots:    rrSlots,
			startIdx: rrStart,
		})
	}
}

func (m StampsModel) syncViewport() StampsModel {
	var content string
	if m.err != "" {
		content = "\n" + errorStyle.Render(m.err)
	} else if m.detailMode && m.cursor < len(m.allSlots) {
		content = m.renderDetail(m.allSlots[m.cursor])
	} else if len(m.allSlots) == 0 {
		content = "\n" + mutedStyle.Render("no stamps yet")
	} else {
		content = m.renderGrid()
	}

	yOffset := m.viewport.YOffset
	m.viewport.SetContent(content)
	if len(m.allSlots) > 0 && !m.detailMode {
		m.viewport.SetYOffset(yOffset)
		// Keep cursor row visible (include category header above first card row)
		cursorRow := m.cursorContentRow()
		scrollTo := cursorRow
		if scrollTo > 0 && m.isCategoryFirstRow() {
			scrollTo-- // show category header
		}
		if scrollTo < m.viewport.YOffset {
			m.viewport.SetYOffset(scrollTo)
		} else if cursorRow+7 >= m.viewport.YOffset+m.viewport.Height {
			m.viewport.SetYOffset(cursorRow + 7 - m.viewport.Height + 1)
		}
	}
	return m
}

// categoryOf returns the category containing the given cursor position and its index.
func (m StampsModel) categoryOf(cursor int) (stampCategory, int) {
	for i, cat := range m.categories {
		if cursor >= cat.startIdx && cursor < cat.startIdx+len(cat.slots) {
			return cat, i
		}
	}
	if len(m.categories) > 0 {
		last := len(m.categories) - 1
		return m.categories[last], last
	}
	return stampCategory{}, 0
}

// isCategoryFirstRow returns true if the cursor is in the first card row of its category.
func (m StampsModel) isCategoryFirstRow() bool {
	for _, cat := range m.categories {
		catEnd := cat.startIdx + len(cat.slots)
		if m.cursor >= cat.startIdx && m.cursor < catEnd {
			return (m.cursor - cat.startIdx) < stampsPerRow
		}
	}
	return false
}

// cursorContentRow returns the approximate line number in content where the cursor's card starts.
func (m StampsModel) cursorContentRow() int {
	row := 0
	for ci, cat := range m.categories {
		if ci > 0 {
			row++ // blank line between categories
		}
		row++ // category header line
		catEnd := cat.startIdx + len(cat.slots)
		if m.cursor >= cat.startIdx && m.cursor < catEnd {
			idxInCat := m.cursor - cat.startIdx
			cardRow := idxInCat / stampsPerRow
			row += cardRow * 8 // each card row is ~8 lines (7 card + 1 gap)
			return row
		}
		cardRows := (len(cat.slots) + stampsPerRow - 1) / stampsPerRow
		row += cardRows * 10
	}
	return row
}

func (m StampsModel) renderGrid() string {
	var b strings.Builder
	for ci, cat := range m.categories {
		if ci > 0 {
			b.WriteString("\n")
		}
		b.WriteString(selectedStyle.Render(cat.name) + "\n")
		for i := 0; i < len(cat.slots); i += stampsPerRow {
			var cards []string
			for j := i; j < i+stampsPerRow && j < len(cat.slots); j++ {
				globalIdx := cat.startIdx + j
				cards = append(cards, renderStampCard(cat.slots[j], globalIdx == m.cursor))
			}
			var withGaps []string
			for k, card := range cards {
				if k > 0 {
					withGaps = append(withGaps, "  ")
				}
				withGaps = append(withGaps, card)
			}
			row := lipgloss.JoinHorizontal(lipgloss.Top, withGaps...)
			for _, line := range strings.Split(row, "\n") {
				b.WriteString(line + "\n")
			}
		}
	}
	return b.String()
}

func (m StampsModel) renderDetail(slot stampSlot) string {
	var b strings.Builder
	b.WriteString("\n")

	fullName := stampDetailName(slot.stampType)
	isRare := strings.HasPrefix(slot.stampType, "rare:")

	b.WriteString(fmt.Sprintf("Stamp     %s %s\n", slot.emoji, selectedStyle.Render(fullName)))

	if slot.stamp != nil {
		rarityLabel := string(slot.stamp.Rarity)
		rarityColor := stampRarityBorderColor(slot.stamp.Rarity)
		b.WriteString(fmt.Sprintf("Rarity    %s\n", lipgloss.NewStyle().Foreground(rarityColor).Render(rarityLabel)))

		if isRare {
			b.WriteString(fmt.Sprintf("Earned    %s\n", mutedStyle.Render(slot.stamp.CreatedAt.Format("Jan 2, 2006"))))
			if desc := rareDescription(slot.stampType); desc != "" {
				b.WriteString(fmt.Sprintf("Award     %s\n", mutedStyle.Render(desc)))
			}
		} else {
			b.WriteString(fmt.Sprintf("Count     %s\n", mutedStyle.Render(fmt.Sprintf("x%d", slot.count))))
		}
	}

	b.WriteString("\n\n" + helpStyle.Render("[b] back to grid"))
	return b.String()
}

// stampDetailName returns the full display name for a stamp in the detail view.
func stampDetailName(stampType string) string {
	if strings.HasPrefix(stampType, "common:") {
		return commonDisplayName(stampType)
	}
	if strings.HasPrefix(stampType, "rare:") {
		return rareDisplayName(stampType)
	}
	if strings.HasPrefix(stampType, "state:") {
		code := strings.ToUpper(strings.TrimPrefix(stampType, "state:"))
		if name, ok := stateNames[code]; ok {
			return name
		}
		return code
	}
	if strings.HasPrefix(stampType, "country:") {
		code := strings.ToUpper(strings.TrimPrefix(stampType, "country:"))
		if name, ok := countryStampNames[code]; ok {
			return name
		}
		return code
	}
	return stampType
}

func rareDescription(key string) string {
	switch key {
	case "rare:cross_country":
		return "Sent a letter from coast to coast"
	case "rare:explorer":
		return "Collected stamps from 10 different states"
	case "rare:penpal":
		return "Exchanged letters with 10 different pen pals"
	case "rare:faithful":
		return "Sent a letter every week for a month"
	case "rare:collector":
		return "Collected 25 unique stamps"
	default:
		return ""
	}
}

func (m StampsModel) View() string {
	m = m.syncViewport()

	title := headerLine(
		titleStyle.Render(fmt.Sprintf("STAMPS (%d)", len(m.stamps))),
		"")
	header := title + "\n" + divider(contentWidth()) + "\n"

	var footer string
	if m.detailMode {
		footer = ""
	} else {
		footer = "\n\n" + helpStyle.Render("[arrows] navigate  [enter] details  [b] back")
	}
	return screenBoxFixed().Render(header + m.viewport.View() + footer)
}

// --- Card rendering ---

func renderStampCard(slot stampSlot, selected bool) string {
	bc := colorDim
	if selected {
		bc = colorAccent
	} else if slot.collected {
		bc = colorBorder
	}

	cardStyle := lipgloss.NewStyle().
		Width(stampCardInnerW).
		Height(5).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(bc)

	var icon string
	if slot.collected {
		icon = slot.emoji
	} else {
		icon = dimStyle.Render("?")
	}
	icon = lipgloss.PlaceHorizontal(stampCardInnerW, lipgloss.Center, icon)

	// Common/rare: emoji only. State/country: keep name + count.
	emojiOnly := strings.HasPrefix(slot.stampType, "common:") || strings.HasPrefix(slot.stampType, "rare:")

	var content string
	if !slot.collected || emojiOnly {
		content = "\n\n" + icon + "\n\n"
	} else {
		name := lipgloss.PlaceHorizontal(stampCardInnerW, lipgloss.Center, slot.displayName)
		content = "\n" + icon + "\n\n" + name + "\n"
	}
	return cardStyle.Render(content)
}

func stampRarityBorderColor(rarity models.StampRarity) lipgloss.AdaptiveColor {
	switch rarity {
	case models.RarityRare:
		return colorAccent
	case models.RarityUltra:
		return colorNew
	default:
		return colorBorder
	}
}

// --- Helpers ---

var abbreviations = map[string]string{
	"North": "N.", "South": "S.", "West": "W.", "New": "N.",
}

func smartTruncate(name string, maxW int) string {
	if lipgloss.Width(name) <= maxW {
		return name
	}
	// Try abbreviating common prefixes
	for long, short := range abbreviations {
		if strings.HasPrefix(name, long+" ") {
			abbr := short + " " + name[len(long)+1:]
			if lipgloss.Width(abbr) <= maxW {
				return abbr
			}
		}
	}
	// Hard truncate
	runes := []rune(name)
	for i := len(runes) - 1; i > 0; i-- {
		candidate := string(runes[:i]) + "…"
		if lipgloss.Width(candidate) <= maxW {
			return candidate
		}
	}
	return string(runes[:1]) + "…"
}

func commonDisplayName(key string) string {
	switch key {
	case "common:flag":
		return "USA"
	case "common:heart":
		return "Heart"
	case "common:star":
		return "Star"
	case "common:quill":
		return "Quill"
	case "common:blossom":
		return "Blossom"
	case "common:sunflower":
		return "Sunflower"
	case "common:butterfly":
		return "Butterfly"
	case "common:wave":
		return "Wave"
	case "common:moon":
		return "Moon"
	case "common:bird":
		return "Bird"
	case "common:rainbow":
		return "Rainbow"
	case "common:clover":
		return "Clover"
	default:
		return key
	}
}

// flagEmojiForCity returns the country flag emoji based on the user's home city.
func flagEmojiForCity(homeCity string) string {
	parts := strings.SplitN(homeCity, ", ", 2)
	if len(parts) == 2 {
		code := strings.TrimSpace(parts[1])
		if emoji, ok := countryEmoji[code]; ok {
			return emoji
		}
	}
	return "🇺🇸"
}

// flagDisplayName returns the country code label for the flag stamp (e.g. "USA", "Spain").
func flagDisplayName(homeCity string) string {
	parts := strings.SplitN(homeCity, ", ", 2)
	if len(parts) == 2 {
		code := strings.TrimSpace(parts[1])
		if name, ok := countryStampNames[code]; ok {
			return name
		}
	}
	return "USA"
}

func rareDisplayName(key string) string {
	switch key {
	case "rare:cross_country":
		return "Cross-Country"
	case "rare:explorer":
		return "Explorer"
	case "rare:penpal":
		return "Pen Pal"
	case "rare:faithful":
		return "Faithful"
	case "rare:collector":
		return "Collector"
	default:
		return key
	}
}

func countCollected(slots []stampSlot) int {
	n := 0
	for _, s := range slots {
		if s.collected {
			n++
		}
	}
	return n
}

// GetStampsAll retrieves all stamps for the user.
func (n *Network) GetStampsAll(ctx context.Context) ([]models.Stamp, error) {
	resp, err := n.Send(ctx, protocol.MsgGetStamps, nil)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(resp.Payload)
	var result protocol.StampsResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing stamps response: %w", err)
	}
	return result.Stamps, nil
}
