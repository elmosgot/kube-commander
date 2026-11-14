package ui

import (
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateLogFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.clearLogFilter()
		m.logScroll = 0
		return m, nil
	case "backspace":
		if m.logFilter != "" {
			r := []rune(m.logFilter)
			m.logFilter = string(r[:len(r)-1])
			m.logScroll = 0
		}
		return m, nil
	case "up":
		return m.moveRight(-1)
	case keyDown:
		return m.moveRight(1)
	case "pgup":
		return m.moveRight(-10)
	case "pgdown":
		return m.moveRight(10)
	}
	return m.appendFilterRunes(msg)
}

// appendFilterRunes adds printable runes to the active filter query.
// Letters (including j/k) append to the query — only arrows scroll while filtering.
// Skip control/meta runes (Alt often injects ESC); keep printable chars so AltGr `|` still works.
func (m Model) appendFilterRunes(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type != tea.KeyRunes || len(msg.Runes) == 0 {
		return m, nil
	}
	var b strings.Builder
	for _, r := range msg.Runes {
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return m, nil
	}
	m.logFilter += b.String()
	m.logScroll = 0
	return m, nil
}

func (m *Model) clearLogFilter() {
	m.logFilterActive = false
	m.logFilter = ""
}

func (m Model) filteredLogLines() []string {
	lines := strings.Split(m.logText, "\n")
	if !m.logFilterActive || m.logFilter == "" {
		return lines
	}
	q := strings.ToLower(m.logFilter)
	exclude := false
	if strings.HasPrefix(q, "!") {
		exclude = true
		q = strings.TrimPrefix(q, "!")
	}
	terms := make([]string, 0)
	for _, part := range strings.Split(q, "|") {
		part = strings.TrimSpace(part)
		if part != "" {
			terms = append(terms, part)
		}
	}
	if len(terms) == 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if logLineMatches(line, terms) != exclude {
			out = append(out, line)
		}
	}
	return out
}

// logLineMatches reports whether line contains any of the lowercase terms.
func logLineMatches(line string, terms []string) bool {
	lower := strings.ToLower(line)
	for _, term := range terms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

// logVisibleCount is how many visual log rows fit in the right pane (matches renderPodLogs).
func (m Model) logVisibleCount() int {
	bodyH, _ := layoutHeights(m.height, m.showCmds)
	visible := bodyH - 4
	if visible < 3 {
		visible = 3
	}
	return visible
}

func (m Model) logContentWidth() int {
	_, rightW := m.paneWidths()
	w := rightW - 4
	if w < 1 {
		return 1
	}
	return w
}

func (m Model) visualLogRows() []visualLogRow {
	return buildVisualLogRows(m.filteredLogLines(), m.logContentWidth())
}

func (m Model) maxLogScroll() int {
	n := len(m.visualLogRows())
	maxStart := n - m.logVisibleCount()
	if maxStart < 0 {
		return 0
	}
	return maxStart
}

// copyLogLogicalLine copies the expanded text of a filtered logical line.
func (m *Model) copyLogLogicalLine(logicalIdx int) {
	lines := m.filteredLogLines()
	if logicalIdx < 0 || logicalIdx >= len(lines) {
		m.status = "clipboard: no line"
		return
	}
	text := expandLogEscapes(lines[logicalIdx])
	if err := copyToClipboard(text); err != nil {
		m.status = "clipboard failed: " + err.Error()
		return
	}
	m.status = "copied"
	m.err = ""
}

// cmdLogIDAtScreen maps a click in the commands panel to a cmd entry id.
func (m Model) cmdLogIDAtScreen(x, y int) (int, bool) {
	if !m.showCmds || m.width == 0 {
		return 0, false
	}
	bodyH, cmdsH := layoutHeights(m.height, m.showCmds)
	if cmdsH < 1 {
		return 0, false
	}
	// y0 header; body; then cmds panel
	panelTop := 1 + bodyH
	if y < panelTop || y >= panelTop+cmdsH {
		return 0, false
	}
	// top border = 0; content from 1
	contentRow := y - panelTop - 1
	if contentRow < 0 {
		return 0, false
	}
	inner := cmdsH - 2
	if inner < 1 {
		inner = 1
	}
	rows := m.visibleCmdPanelRows(inner)
	if contentRow >= len(rows) {
		return 0, false
	}
	r := rows[contentRow]
	if r.kind == rowKindTitle || r.entryID == 0 {
		return 0, false
	}
	return r.entryID, true
}

// visibleCmdPanelRows returns the title plus the tail of command rows that fit.
func (m Model) visibleCmdPanelRows(inner int) []cmdPanelRow {
	rows := buildCmdPanelRows(m.cmdLog, m.cmdAnimFrame, time.Now(), m.width)
	if len(rows) <= inner {
		return rows
	}
	keep := inner - 1
	if keep < 0 {
		keep = 0
	}
	tail := rows[1:]
	if len(tail) > keep {
		tail = tail[len(tail)-keep:]
	}
	return append([]cmdPanelRow{rows[0]}, tail...)
}

// logLogicalIdxAtScreen maps terminal mouse coords to a filtered logical line index.
func (m Model) logLogicalIdxAtScreen(x, y int) (int, bool) {
	if m.rightView != viewPodLogs || m.width == 0 {
		return 0, false
	}
	leftW, _ := m.paneWidths()
	if x < leftW {
		return 0, false
	}
	bodyH, _ := layoutHeights(m.height, m.showCmds)
	// Layout: y0 header, then bodyH rows of panes.
	if y < 1 || y >= 1+bodyH {
		return 0, false
	}
	// Right pane: top border (0), title (1), log rows from 2.
	contentRow := y - 1 - 2
	if contentRow < 0 || contentRow >= m.logVisibleCount() {
		return 0, false
	}
	return logLineAtVisualOffset(m.visualLogRows(), m.logScroll, contentRow)
}

func (m *Model) scrollLogsToBottom() {
	m.logScroll = m.maxLogScroll()
	m.logStickBottom = true
}
