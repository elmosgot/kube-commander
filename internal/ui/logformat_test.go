package ui

import (
	"strings"
	"testing"
)

func TestExpandLogEscapes(t *testing.T) {
	got := expandLogEscapes(`a\nb\tc`)
	if got != "a\nb\tc" {
		t.Fatalf("got %q", got)
	}
}

func TestWrapLogLine(t *testing.T) {
	parts := wrapLogLine("abcdefghij", 4)
	if len(parts) != 3 || parts[0] != "abcd" || parts[1] != "efgh" || parts[2] != "ij" {
		t.Fatalf("parts = %#v", parts)
	}
}

func TestBuildVisualLogRowsEscapesAndWrap(t *testing.T) {
	rows := buildVisualLogRows([]string{`ab\ncd`, "efghij"}, 4)
	if len(rows) != 4 {
		t.Fatalf("rows = %#v", rows)
	}
	if rows[0].text != "ab" || rows[0].logicalIdx != 0 {
		t.Fatalf("row0 = %#v", rows[0])
	}
	if rows[1].text != "cd" || rows[1].logicalIdx != 0 {
		t.Fatalf("row1 = %#v", rows[1])
	}
	if rows[2].text != "efgh" || rows[2].logicalIdx != 1 {
		t.Fatalf("row2 = %#v", rows[2])
	}
	if rows[3].text != "ij" || rows[3].logicalIdx != 1 {
		t.Fatalf("row3 = %#v", rows[3])
	}
}

func TestLogLineAtVisualOffset(t *testing.T) {
	rows := buildVisualLogRows([]string{`ab\ncd`, "ef"}, 10)
	idx, ok := logLineAtVisualOffset(rows, 0, 1)
	if !ok || idx != 0 {
		t.Fatalf("idx=%d ok=%v", idx, ok)
	}
	idx, ok = logLineAtVisualOffset(rows, 1, 1)
	if !ok || idx != 1 {
		t.Fatalf("idx=%d ok=%v", idx, ok)
	}
	_, ok = logLineAtVisualOffset(rows, 0, 99)
	if ok {
		t.Fatal("expected miss")
	}
}

func TestRenderPodLogsWrapsWithoutEllipsis(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 80
	m.height = 30
	m.rightView = viewPodLogs
	m.logText = `{"msg":"one\ntwo"}` + "\nshort"

	var b strings.Builder
	m.renderPodLogs(&b, 40, 20)
	out := b.String()
	if strings.Contains(out, "…") {
		t.Fatalf("unexpected ellipsis:\n%s", out)
	}
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		t.Fatalf("expected expanded wrap:\n%s", out)
	}
}

func TestLogLogicalIdxAtScreen(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30
	m.rightView = viewPodLogs
	m.logText = "alpha\nbeta\ngamma"
	m.logScroll = 0

	leftW, _ := m.paneWidths()
	// header=0, body starts 1; border+title → content at y=3
	idx, ok := m.logLogicalIdxAtScreen(leftW+2, 3)
	if !ok || idx != 0 {
		t.Fatalf("first line idx=%d ok=%v", idx, ok)
	}
	idx, ok = m.logLogicalIdxAtScreen(leftW+2, 4)
	if !ok || idx != 1 {
		t.Fatalf("second line idx=%d ok=%v", idx, ok)
	}
	_, ok = m.logLogicalIdxAtScreen(0, 3)
	if ok {
		t.Fatal("left pane should miss")
	}
}
