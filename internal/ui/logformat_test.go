package ui

import (
	"strings"
	"testing"
)

func TestExpandLogEscapes(test *testing.T) {
	got := expandLogEscapes(`a\nb\tc`)
	if got != "a\nb\tc" {
		test.Fatalf("got %q", got)
	}
}

func TestWrapLogLine(test *testing.T) {
	parts := wrapLogLine("abcdefghij", 4)
	if len(parts) != 3 || parts[0] != "abcd" || parts[1] != "efgh" || parts[2] != "ij" {
		test.Fatalf("parts = %#v", parts)
	}
}

func TestBuildVisualLogRowsEscapesAndWrap(test *testing.T) {
	rows := buildVisualLogRows([]string{`ab\ncd`, "efghij"}, 4)
	if len(rows) != 4 {
		test.Fatalf("rows = %#v", rows)
	}
	if rows[0].text != "ab" || rows[0].logicalIdx != 0 {
		test.Fatalf("row0 = %#v", rows[0])
	}
	if rows[1].text != "cd" || rows[1].logicalIdx != 0 {
		test.Fatalf("row1 = %#v", rows[1])
	}
	if rows[2].text != "efgh" || rows[2].logicalIdx != 1 {
		test.Fatalf("row2 = %#v", rows[2])
	}
	if rows[3].text != "ij" || rows[3].logicalIdx != 1 {
		test.Fatalf("row3 = %#v", rows[3])
	}
}

func TestLogLineAtVisualOffset(test *testing.T) {
	rows := buildVisualLogRows([]string{`ab\ncd`, "ef"}, 10)
	idx, ok := logLineAtVisualOffset(rows, 0, 1)
	if !ok || idx != 0 {
		test.Fatalf("idx=%d ok=%v", idx, ok)
	}
	idx, ok = logLineAtVisualOffset(rows, 1, 1)
	if !ok || idx != 1 {
		test.Fatalf("idx=%d ok=%v", idx, ok)
	}
	_, ok = logLineAtVisualOffset(rows, 0, 99)
	if ok {
		test.Fatal("expected miss")
	}
}

func TestRenderPodLogsWrapsWithoutEllipsis(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 80
	model.height = 30
	model.rightView = viewPodLogs
	model.logText = `{"msg":"one\ntwo"}` + "\nshort"

	var b strings.Builder
	model.renderPodLogs(&b, 40, 20)
	out := b.String()
	if strings.Contains(out, "…") {
		test.Fatalf("unexpected ellipsis:\n%s", out)
	}
	if !strings.Contains(out, "one") || !strings.Contains(out, "two") {
		test.Fatalf("expected expanded wrap:\n%s", out)
	}
}

func TestLogLogicalIdxAtScreen(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30
	model.rightView = viewPodLogs
	model.logText = "alpha\nbeta\ngamma"
	model.logScroll = 0

	leftW, _ := model.paneWidths()
	// header=0, body starts 1; border+title → content at y=3
	idx, ok := model.logLogicalIdxAtScreen(leftW+2, 3)
	if !ok || idx != 0 {
		test.Fatalf("first line idx=%d ok=%v", idx, ok)
	}
	idx, ok = model.logLogicalIdxAtScreen(leftW+2, 4)
	if !ok || idx != 1 {
		test.Fatalf("second line idx=%d ok=%v", idx, ok)
	}
	_, ok = model.logLogicalIdxAtScreen(0, 3)
	if ok {
		test.Fatal("left pane should miss")
	}
}
