package ui

import (
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

func (model Model) updateLogFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		model.clearLogFilter()
		model.logScroll = 0
		return model, nil
	case "backspace":
		if model.logFilter != "" {
			r := []rune(model.logFilter)
			model.logFilter = string(r[:len(r)-1])
			model.logScroll = 0
		}
		return model, nil
	case "up":
		return model.moveRight(-1)
	case keyDown:
		return model.moveRight(1)
	case "pgup":
		return model.moveRight(-10)
	case "pgdown":
		return model.moveRight(10)
	}
	return model.appendFilterRunes(msg)
}

// appendFilterRunes adds printable runes to the active filter query.
// Letters (including j/k) append to the query — only arrows scroll while filtering.
// Skip control/meta runes (Alt often injects ESC); keep printable chars so AltGr `|` still works.
func (model Model) appendFilterRunes(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type != tea.KeyRunes || len(msg.Runes) == 0 {
		return model, nil
	}
	var buf strings.Builder
	for _, r := range msg.Runes {
		if unicode.IsPrint(r) {
			buf.WriteRune(r)
		}
	}
	if buf.Len() == 0 {
		return model, nil
	}
	model.logFilter += buf.String()
	model.logScroll = 0
	return model, nil
}

func (model *Model) clearLogFilter() {
	model.logFilterActive = false
	model.logFilter = ""
}

func (model Model) filteredLogLines() []string {
	lines := strings.Split(model.logText, "\n")
	if !model.logFilterActive || model.logFilter == "" {
		return lines
	}
	query := strings.ToLower(model.logFilter)
	exclude := false
	if strings.HasPrefix(query, "!") {
		exclude = true
		query = strings.TrimPrefix(query, "!")
	}
	terms := make([]string, 0)
	for _, part := range strings.Split(query, "|") {
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
func (model Model) logVisibleCount() int {
	bodyH, _ := layoutHeights(model.height, model.showCmds)
	visible := bodyH - 4
	if visible < 3 {
		visible = 3
	}
	return visible
}

func (model Model) logContentWidth() int {
	_, rightW := model.paneWidths()
	w := rightW - 4
	if w < 1 {
		return 1
	}
	return w
}

func (model Model) visualLogRows() []visualLogRow {
	return buildVisualLogRows(model.filteredLogLines(), model.logContentWidth())
}

func (model Model) maxLogScroll() int {
	n := len(model.visualLogRows())
	maxStart := n - model.logVisibleCount()
	if maxStart < 0 {
		return 0
	}
	return maxStart
}

// copyLogLogicalLine copies the expanded text of a filtered logical line.
func (model *Model) copyLogLogicalLine(logicalIdx int) {
	lines := model.filteredLogLines()
	if logicalIdx < 0 || logicalIdx >= len(lines) {
		model.status = "clipboard: no line"
		return
	}
	text := expandLogEscapes(lines[logicalIdx])
	if err := copyToClipboard(text); err != nil {
		model.status = "clipboard failed: " + err.Error()
		return
	}
	model.status = "copied"
	model.err = ""
}

// cmdLogIDAtScreen maps a click in the commands panel to a cmd entry id.
func (model Model) cmdLogIDAtScreen(screenX, screenY int) (int, bool) {
	if !model.showCmds || model.width == 0 {
		return 0, false
	}
	bodyH, cmdsH := layoutHeights(model.height, model.showCmds)
	if cmdsH < 1 {
		return 0, false
	}
	// y0 header; body; then cmds panel
	panelTop := 1 + bodyH
	if screenY < panelTop || screenY >= panelTop+cmdsH {
		return 0, false
	}
	// top border = 0; content from 1
	contentRow := screenY - panelTop - 1
	if contentRow < 0 {
		return 0, false
	}
	inner := cmdsH - 2
	if inner < 1 {
		inner = 1
	}
	rows := model.visibleCmdPanelRows(inner)
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
func (model Model) visibleCmdPanelRows(inner int) []cmdPanelRow {
	rows := buildCmdPanelRows(model.cmdLog, model.cmdAnimFrame, time.Now(), model.width)
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
func (model Model) logLogicalIdxAtScreen(screenX, screenY int) (int, bool) {
	if model.rightView != viewPodLogs || model.width == 0 {
		return 0, false
	}
	leftW, _ := model.paneWidths()
	if screenX < leftW {
		return 0, false
	}
	bodyH, _ := layoutHeights(model.height, model.showCmds)
	// Layout: y0 header, then bodyH rows of panes.
	if screenY < 1 || screenY >= 1+bodyH {
		return 0, false
	}
	// Right pane: top border (0), title (1), log rows from 2.
	contentRow := screenY - 1 - 2
	if contentRow < 0 || contentRow >= model.logVisibleCount() {
		return 0, false
	}
	return logLineAtVisualOffset(model.visualLogRows(), model.logScroll, contentRow)
}

func (model *Model) scrollLogsToBottom() {
	model.logScroll = model.maxLogScroll()
	model.logStickBottom = true
}
