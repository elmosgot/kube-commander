package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLogFilterActivateAndMatch(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30
	model.leftCursor = 1
	model.focus = paneRight
	model.rightView = viewPodLogs
	model.logText = "alpha line\nbeta error\ngamma line\n"

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model = updated.(Model)
	if !model.logFilterActive {
		test.Fatal("expected filter active after f")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e', 'r', 'r'}})
	model = updated.(Model)
	if model.logFilter != tErr {
		test.Fatalf("filter = %q", model.logFilter)
	}
	lines := model.filteredLogLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "error") {
		test.Fatalf("filtered = %v", lines)
	}
	if model.rightLen() != 1 {
		test.Fatalf("rightLen = %d", model.rightLen())
	}
}

func TestLogFilterSlashAndEsc(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.focus = paneRight
	model.rightView = viewPodLogs
	model.logText = "one\ntwo\n"

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)
	if !model.logFilterActive {
		test.Fatal("expected filter active after /")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	model = updated.(Model)
	if len(model.filteredLogLines()) != 1 {
		test.Fatalf("lines = %v", model.filteredLogLines())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEscape})
	model = updated.(Model)
	if model.logFilterActive || model.logFilter != "" {
		test.Fatalf("filter should clear: active=%v filter=%q", model.logFilterActive, model.logFilter)
	}
	if model.rightView != viewPodLogs {
		test.Fatal("esc should stay on logs")
	}
}

func TestLogFilterBackspaceEditsQuery(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.focus = paneRight
	model.rightView = viewPodLogs
	model.logFilterActive = true
	model.logFilter = "ab"
	model.logText = "ab\nac\n"

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(Model)
	if model.logFilter != "a" {
		test.Fatalf("filter = %q", model.logFilter)
	}
	if model.rightView != viewPodLogs {
		test.Fatal("backspace should not leave logs while filtering")
	}
}

func TestLogFilterCapturesGlobalHotkeys(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30
	model.focus = paneRight
	model.rightView = viewPodLogs
	model.logFilterActive = true
	model.logFilter = "do"
	model.logText = "doctrine\nother\n"
	model.showCmds = false

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)
	if model.logFilter != "doc" {
		test.Fatalf("filter = %q want doc", model.logFilter)
	}
	if model.showCmds {
		test.Fatal("c must not toggle commands while filtering")
	}
	if cmd != nil {
		test.Fatal("r/c must not trigger refresh/other cmds while filtering")
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model = updated.(Model)
	if model.logFilter != "docr" {
		test.Fatalf("filter = %q want docr", model.logFilter)
	}
	if cmd != nil {
		test.Fatal("r must not refresh while filtering")
	}
}

func TestLogFilterHelpOnBottom(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30
	model.focus = paneRight
	model.rightView = viewPodLogs
	model.logFilterActive = true
	model.logFilter = "foo"
	model.logText = "foo\nbar\n"

	view := model.View()
	if !strings.Contains(view, "Filter: foo") {
		test.Fatalf("missing bottom filter prompt:\n%s", view)
	}
	// Filter prompt should not appear as a logs-pane header line only — help line carries it.
	help := model.renderHelp()
	if !strings.Contains(help, "Filter: foo█") {
		test.Fatalf("help = %q", help)
	}
	if !strings.Contains(help, "! exclude") {
		test.Fatalf("help missing exclude hint: %q", help)
	}
	if !strings.Contains(help, "| or") {
		test.Fatalf("help missing OR hint: %q", help)
	}
}

func TestLogFilterExcludePrefix(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.logFilterActive = true
	model.logText = "GET /ready\nPOST /orders\nGET /readyz\nERROR boom"

	model.logFilter = tErr
	lines := model.filteredLogLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "ERROR") {
		test.Fatalf("positive filter = %v", lines)
	}

	model.logFilter = "!ready"
	lines = model.filteredLogLines()
	if len(lines) != 2 {
		test.Fatalf("exclude ready: got %v", lines)
	}
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "ready") {
			test.Fatalf("exclude leaked ready line: %q", line)
		}
	}

	model.logFilter = "!"
	lines = model.filteredLogLines()
	if len(lines) != 4 {
		test.Fatalf("bare ! should show all, got %v", lines)
	}
}

func TestLogFilterIgnoresControlRunes(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.focus = paneRight
	model.rightView = viewPodLogs
	model.logFilterActive = true
	model.logFilter = tErr
	model.logText = "error\nready\n"

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x1b}}) // ESC from Alt
	model = updated.(Model)
	if model.logFilter != tErr {
		test.Fatalf("control rune altered filter: %q", model.logFilter)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x1b, '|'}})
	model = updated.(Model)
	if model.logFilter != "err|" {
		test.Fatalf("expected printable | only, got %q", model.logFilter)
	}
}

func TestLogFilterOR(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.logFilterActive = true
	model.logText = "GET /ready\nPOST /orders\nWARN slow\nERROR boom"

	model.logFilter = "error|warn"
	lines := model.filteredLogLines()
	if len(lines) != 2 {
		test.Fatalf("OR filter = %v", lines)
	}

	model.logFilter = "error|"
	lines = model.filteredLogLines()
	if len(lines) != 1 || !strings.Contains(lines[0], "ERROR") {
		test.Fatalf("trailing pipe = %v", lines)
	}

	model.logFilter = "|"
	lines = model.filteredLogLines()
	if len(lines) != 4 {
		test.Fatalf("bare | should show all, got %v", lines)
	}

	model.logFilter = "!ready|warn"
	lines = model.filteredLogLines()
	// exclude if line matches ready OR warn → keep POST and ERROR
	if len(lines) != 2 {
		test.Fatalf("exclude OR = %v", lines)
	}
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "ready") || strings.Contains(lower, "warn") {
			test.Fatalf("exclude OR leaked: %q", line)
		}
	}
}
