package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elmosgot/kube-commander/internal/kube"
)

func TestAutoRefreshSchedulesQuietFetch(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30
	m.focus = paneRight
	m.rightView = viewWorkloads
	m.namespace = "app"
	m.workloads = []kube.WorkloadInfo{{Name: "api"}}
	m.rightCursor = 0
	m.available[envKey(m.cfg.Pools[0].Environments[0])] = true
	m.loading = false
	m.autoRefreshBusy = false

	updated, cmd := m.Update(autoRefreshMsg{})
	m = updated.(Model)
	if !m.autoRefreshBusy {
		t.Fatal("expected autoRefreshBusy")
	}
	if cmd == nil {
		t.Fatal("expected batch cmd")
	}
}

func TestAutoRefreshSkipsLogsView(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.rightView = viewPodLogs
	m.loading = false
	m.autoRefreshBusy = false

	updated, cmd := m.Update(autoRefreshMsg{})
	m = updated.(Model)
	if m.autoRefreshBusy {
		t.Fatal("logs view should not start quiet refresh")
	}
	if cmd == nil {
		t.Fatal("expected tick re-arm cmd")
	}
}

func TestSilentWorkloadsPreservesCursorAndFilterIrrelevant(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.focus = paneRight
	m.rightView = viewWorkloads
	m.namespace = "app"
	m.workloads = []kube.WorkloadInfo{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m.rightCursor = 2

	updated, _ := m.Update(workloadsMsg{
		poolIdx:   0,
		envIdx:    0,
		namespace: "app",
		items:     []kube.WorkloadInfo{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}},
		silent:    true,
	})
	m = updated.(Model)
	if m.rightCursor != 2 {
		t.Fatalf("cursor = %d, want 2", m.rightCursor)
	}
	if len(m.workloads) != 4 {
		t.Fatalf("len = %d", len(m.workloads))
	}
}

func TestLogChunkAppendsWithoutClearingFilter(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30
	m.focus = paneRight
	m.rightView = viewPodLogs
	m.logFollowID = 3
	m.logText = "alpha\n"
	m.logFilterActive = true
	m.logFilter = "err"
	m.logStickBottom = true
	m.logScroll = 0

	ch := make(chan tea.Msg)
	updated, cmd := m.Update(logChunkMsg{id: 3, text: "error line", ch: ch})
	m = updated.(Model)
	if !m.logFilterActive || m.logFilter != "err" {
		t.Fatalf("filter cleared: active=%v filter=%q", m.logFilterActive, m.logFilter)
	}
	if m.logText != "alpha\nerror line" {
		t.Fatalf("logText = %q", m.logText)
	}
	if cmd == nil {
		t.Fatal("expected listen cmd")
	}
}

func TestLogChunkIgnoresStaleID(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.rightView = viewPodLogs
	m.logFollowID = 5
	m.logText = "keep"
	m.logFilterActive = true
	m.logFilter = "x"

	ch := make(chan tea.Msg)
	updated, cmd := m.Update(logChunkMsg{id: 4, text: "stale", ch: ch})
	m = updated.(Model)
	if m.logText != "keep" {
		t.Fatalf("logText = %q", m.logText)
	}
	if m.logFilter != "x" {
		t.Fatal("filter changed")
	}
	if cmd == nil {
		t.Fatal("should still drain stale channel")
	}
}

func TestStopLogFollowIncrementsID(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.logFollowID = 1
	canceled := false
	m.logFollowCancel = func() { canceled = true }
	m.stopLogFollow()
	if !canceled {
		t.Fatal("expected cancel")
	}
	if m.logFollowID != 2 {
		t.Fatalf("id = %d", m.logFollowID)
	}
	if m.logFollowCancel != nil {
		t.Fatal("cancel should be cleared")
	}
}

func TestScheduleAutoRefreshInterval(t *testing.T) {
	if autoRefreshInterval != 5*time.Second {
		t.Fatalf("interval = %s", autoRefreshInterval)
	}
}
