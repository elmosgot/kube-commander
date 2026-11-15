package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elmosgot/kube-commander/internal/kube"
)

func TestAutoRefreshSchedulesQuietFetch(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30
	model.focus = paneRight
	model.rightView = viewWorkloads
	model.namespace = tApp
	model.workloads = []kube.WorkloadInfo{{Name: tAPI}}
	model.rightCursor = 0
	model.available[envKey(model.cfg.Pools[0].Environments[0])] = true
	model.loading = false
	model.autoRefreshBusy = false

	updated, cmd := model.Update(autoRefreshMsg{})
	model = updated.(Model)
	if !model.autoRefreshBusy {
		test.Fatal("expected autoRefreshBusy")
	}
	if cmd == nil {
		test.Fatal("expected batch cmd")
	}
}

func TestAutoRefreshSkipsLogsView(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.rightView = viewPodLogs
	model.loading = false
	model.autoRefreshBusy = false

	updated, cmd := model.Update(autoRefreshMsg{})
	model = updated.(Model)
	if model.autoRefreshBusy {
		test.Fatal("logs view should not start quiet refresh")
	}
	if cmd == nil {
		test.Fatal("expected tick re-arm cmd")
	}
}

func TestSilentWorkloadsPreservesCursorAndFilterIrrelevant(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.focus = paneRight
	model.rightView = viewWorkloads
	model.namespace = tApp
	model.workloads = []kube.WorkloadInfo{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	model.rightCursor = 2

	updated, _ := model.Update(workloadsMsg{
		poolIdx:   0,
		envIdx:    0,
		namespace: tApp,
		items:     []kube.WorkloadInfo{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}},
		silent:    true,
	})
	model = updated.(Model)
	if model.rightCursor != 2 {
		test.Fatalf("cursor = %d, want 2", model.rightCursor)
	}
	if len(model.workloads) != 4 {
		test.Fatalf("len = %d", len(model.workloads))
	}
}

func TestLogChunkAppendsWithoutClearingFilter(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30
	model.focus = paneRight
	model.rightView = viewPodLogs
	model.logFollowID = 3
	model.logText = "alpha\n"
	model.logFilterActive = true
	model.logFilter = tErr
	model.logStickBottom = true
	model.logScroll = 0

	ch := make(chan tea.Msg)
	updated, cmd := model.Update(logChunkMsg{id: 3, text: "error line", ch: ch})
	model = updated.(Model)
	if !model.logFilterActive || model.logFilter != tErr {
		test.Fatalf("filter cleared: active=%v filter=%q", model.logFilterActive, model.logFilter)
	}
	if model.logText != "alpha\nerror line" {
		test.Fatalf("logText = %q", model.logText)
	}
	if cmd == nil {
		test.Fatal("expected listen cmd")
	}
}

func TestLogChunkIgnoresStaleID(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.rightView = viewPodLogs
	model.logFollowID = 5
	model.logText = "keep"
	model.logFilterActive = true
	model.logFilter = "x"

	ch := make(chan tea.Msg)
	updated, cmd := model.Update(logChunkMsg{id: 4, text: "stale", ch: ch})
	model = updated.(Model)
	if model.logText != "keep" {
		test.Fatalf("logText = %q", model.logText)
	}
	if model.logFilter != "x" {
		test.Fatal("filter changed")
	}
	if cmd == nil {
		test.Fatal("should still drain stale channel")
	}
}

func TestStopLogFollowIncrementsID(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.logFollowID = 1
	canceled := false
	model.logFollowCancel = func() { canceled = true }
	model.stopLogFollow()
	if !canceled {
		test.Fatal("expected cancel")
	}
	if model.logFollowID != 2 {
		test.Fatalf("id = %d", model.logFollowID)
	}
	if model.logFollowCancel != nil {
		test.Fatal("cancel should be cleared")
	}
}

func TestScheduleAutoRefreshInterval(test *testing.T) {
	if autoRefreshInterval != 5*time.Second {
		test.Fatalf("interval = %s", autoRefreshInterval)
	}
}
