package ui

import (
	"strings"
)

// visualLogRow is one terminal row produced from a filtered logical log line.
type visualLogRow struct {
	text       string
	logicalIdx int
}

// expandLogEscapes turns literal \n \t \r sequences into real control characters.
func expandLogEscapes(s string) string {
	return strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\r`, "\r").Replace(s)
}

// wrapLogLine wraps s to at most width runes per row (width < 1 treated as 1).
func wrapLogLine(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	if s == "" {
		return []string{""}
	}
	runes := []rune(s)
	out := make([]string, 0, (len(runes)+width-1)/width)
	for len(runes) > 0 {
		n := width
		if n > len(runes) {
			n = len(runes)
		}
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return out
}

// buildVisualLogRows expands escapes and wraps each logical line into display rows.
func buildVisualLogRows(logical []string, width int) []visualLogRow {
	if width < 1 {
		width = 1
	}
	out := make([]visualLogRow, 0, len(logical))
	for i, line := range logical {
		expanded := expandLogEscapes(line)
		parts := strings.Split(expanded, "\n")
		for _, part := range parts {
			for _, w := range wrapLogLine(part, width) {
				out = append(out, visualLogRow{text: w, logicalIdx: i})
			}
		}
	}
	return out
}

// logLineAtVisualOffset returns the logical index for a visual row at scroll+offset.
func logLineAtVisualOffset(rows []visualLogRow, scroll, offset int) (logicalIdx int, ok bool) {
	idx := scroll + offset
	if idx < 0 || idx >= len(rows) {
		return 0, false
	}
	return rows[idx].logicalIdx, true
}
