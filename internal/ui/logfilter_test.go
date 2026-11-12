package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLogFilterActivateAndMatch(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30
	m.leftCursor = 1
	m.focus = paneRight
	m.rightView = viewPodLogs
	m.logText = "alpha line\nbeta error\ngamma line\n"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m = updated.(Model)
	if !m.logFilterActive {
		t.Fatal("expected filter active after f")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e', 'r', 'r'}})
	m = updated.(Model)
	if m.logFilter != "err" {
		t.Fatalf("filter = %q", m.logFilter)
	}
	lines := m.filteredLogLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "error") {
		t.Fatalf("filtered = %v", lines)
	}
	if m.rightLen() != 1 {
		t.Fatalf("rightLen = %d", m.rightLen())
	}
}

func TestLogFilterSlashAndEsc(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.focus = paneRight
	m.rightView = viewPodLogs
	m.logText = "one\ntwo\n"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(Model)
	if !m.logFilterActive {
		t.Fatal("expected filter active after /")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = updated.(Model)
	if len(m.filteredLogLines()) != 1 {
		t.Fatalf("lines = %v", m.filteredLogLines())
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(Model)
	if m.logFilterActive || m.logFilter != "" {
		t.Fatalf("filter should clear: active=%v filter=%q", m.logFilterActive, m.logFilter)
	}
	if m.rightView != viewPodLogs {
		t.Fatal("esc should stay on logs")
	}
}

func TestLogFilterBackspaceEditsQuery(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.focus = paneRight
	m.rightView = viewPodLogs
	m.logFilterActive = true
	m.logFilter = "ab"
	m.logText = "ab\nac\n"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(Model)
	if m.logFilter != "a" {
		t.Fatalf("filter = %q", m.logFilter)
	}
	if m.rightView != viewPodLogs {
		t.Fatal("backspace should not leave logs while filtering")
	}
}

func TestLogFilterCapturesGlobalHotkeys(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30
	m.focus = paneRight
	m.rightView = viewPodLogs
	m.logFilterActive = true
	m.logFilter = "do"
	m.logText = "doctrine\nother\n"
	m.showCmds = false

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updated.(Model)
	if m.logFilter != "doc" {
		t.Fatalf("filter = %q want doc", m.logFilter)
	}
	if m.showCmds {
		t.Fatal("c must not toggle commands while filtering")
	}
	if cmd != nil {
		t.Fatal("r/c must not trigger refresh/other cmds while filtering")
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(Model)
	if m.logFilter != "docr" {
		t.Fatalf("filter = %q want docr", m.logFilter)
	}
	if cmd != nil {
		t.Fatal("r must not refresh while filtering")
	}
}

func TestLogFilterHelpOnBottom(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30
	m.focus = paneRight
	m.rightView = viewPodLogs
	m.logFilterActive = true
	m.logFilter = "foo"
	m.logText = "foo\nbar\n"

	view := m.View()
	if !strings.Contains(view, "Filter: foo") {
		t.Fatalf("missing bottom filter prompt:\n%s", view)
	}
	// Filter prompt should not appear as a logs-pane header line only — help line carries it.
	help := m.renderHelp()
	if !strings.Contains(help, "Filter: foo█") {
		t.Fatalf("help = %q", help)
	}
	if !strings.Contains(help, "! exclude") {
		t.Fatalf("help missing exclude hint: %q", help)
	}
	if !strings.Contains(help, "| or") {
		t.Fatalf("help missing OR hint: %q", help)
	}
}

func TestLogFilterExcludePrefix(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.logFilterActive = true
	m.logText = "GET /ready\nPOST /orders\nGET /readyz\nERROR boom"

	m.logFilter = "err"
	lines := m.filteredLogLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "ERROR") {
		t.Fatalf("positive filter = %v", lines)
	}

	m.logFilter = "!ready"
	lines = m.filteredLogLines()
	if len(lines) != 2 {
		t.Fatalf("exclude ready: got %v", lines)
	}
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "ready") {
			t.Fatalf("exclude leaked ready line: %q", line)
		}
	}

	m.logFilter = "!"
	lines = m.filteredLogLines()
	if len(lines) != 4 {
		t.Fatalf("bare ! should show all, got %v", lines)
	}
}

func TestLogFilterIgnoresControlRunes(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.focus = paneRight
	m.rightView = viewPodLogs
	m.logFilterActive = true
	m.logFilter = "err"
	m.logText = "error\nready\n"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x1b}}) // ESC from Alt
	m = updated.(Model)
	if m.logFilter != "err" {
		t.Fatalf("control rune altered filter: %q", m.logFilter)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x1b, '|'}})
	m = updated.(Model)
	if m.logFilter != "err|" {
		t.Fatalf("expected printable | only, got %q", m.logFilter)
	}
}

func TestLogFilterOR(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.logFilterActive = true
	m.logText = "GET /ready\nPOST /orders\nWARN slow\nERROR boom"

	m.logFilter = "error|warn"
	lines := m.filteredLogLines()
	if len(lines) != 2 {
		t.Fatalf("OR filter = %v", lines)
	}

	m.logFilter = "error|"
	lines = m.filteredLogLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "ERROR") {
		t.Fatalf("trailing pipe = %v", lines)
	}

	m.logFilter = "|"
	lines = m.filteredLogLines()
	if len(lines) != 4 {
		t.Fatalf("bare | should show all, got %v", lines)
	}

	m.logFilter = "!ready|warn"
	lines = m.filteredLogLines()
	// exclude if line matches ready OR warn → keep POST and ERROR
	if len(lines) != 2 {
		t.Fatalf("exclude OR = %v", lines)
	}
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "ready") || strings.Contains(lower, "warn") {
			t.Fatalf("exclude OR leaked: %q", line)
		}
	}
}
