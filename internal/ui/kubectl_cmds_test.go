package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elmosgot/kube-commander/internal/kube"
)

func TestKubectlLineBuilders(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.leftCursor = 1
	m.namespace = "app"
	m.deployment = "api"
	m.podName = "api-1"
	m.rightView = viewWorkloads
	if got := m.kubectlLineWorkloads("app"); !strings.Contains(got, "-n app get deployments") {
		t.Fatalf("workloads: %s", got)
	}
	m.rightView = viewPods
	if got := m.kubectlLinePods("app", "api"); !strings.Contains(got, "get pods") {
		t.Fatalf("pods: %s", got)
	}
	m.podDetail = &kube.PodDetail{
		Name:       "api-1",
		Containers: []kube.ContainerInfo{{Name: "app"}},
	}
	m.rightView = viewPodLogs
	if got := m.kubectlLogsFollowLine(); !strings.Contains(got, "logs api-1 --tail=200 --follow") {
		t.Fatalf("logs: %s", got)
	}
}

func TestToggleCommandsPanel(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 40
	m.leftCursor = 1
	m.beginCmd("kubectl get pods", false)

	if m.showCmds {
		t.Fatal("expected hidden by default")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updated.(Model)
	if !m.showCmds {
		t.Fatal("expected showCmds after c")
	}
	view := m.View()
	if !strings.Contains(view, "Commands") {
		t.Fatalf("panel missing title:\n%s", view)
	}
	if strings.Contains(view, "\nView\n") || strings.Contains(view, "Shell") {
		t.Fatalf("old View/Shell sections still present:\n%s", view)
	}
	if !strings.Contains(view, "kubectl get pods") {
		t.Fatalf("panel missing live command:\n%s", view)
	}
}

func TestLayoutHeightsFitsTerminal(t *testing.T) {
	bodyH, cmdsH := layoutHeights(24, true)
	if bodyH+cmdsH+3 > 24 { // + chrome
		t.Fatalf("overflow: body=%d cmds=%d", bodyH, cmdsH)
	}
	if bodyH < 1 || cmdsH < 1 {
		t.Fatalf("expected positive body/cmds, got body=%d cmds=%d", bodyH, cmdsH)
	}

	bodyH, cmdsH = layoutHeights(10, true)
	if bodyH+cmdsH+3 > 10 {
		t.Fatalf("tight overflow: body=%d cmds=%d", bodyH, cmdsH)
	}
	if bodyH != 1 {
		t.Fatalf("expected bodyH=1 on tight terminal, got %d (cmds=%d)", bodyH, cmdsH)
	}

	bodyH, cmdsH = layoutHeights(40, false)
	if cmdsH != 0 || bodyH != 37 {
		t.Fatalf("hidden cmds: body=%d cmds=%d", bodyH, cmdsH)
	}
}

func TestCommandsPanelViewFitsHeight(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 80
	m.height = 16
	m.leftCursor = 1
	m.showCmds = true
	m.beginCmd("kubectl get namespaces", false)

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) > m.height+2 {
		t.Fatalf("view has %d lines for height %d:\n%s", len(lines), m.height, view)
	}
}

func TestCmdLogBeginFinish(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	id := m.beginCmd("kubectl get pods", false)
	if len(m.cmdLog) != 1 || !m.cmdLog[0].Running {
		t.Fatalf("entry = %+v", m.cmdLog)
	}
	time.Sleep(2 * time.Millisecond)
	m.finishCmd(id, nil)
	if m.cmdLog[0].Running {
		t.Fatal("still running")
	}
	if m.cmdLog[0].Ended.Before(m.cmdLog[0].Started) {
		t.Fatal("bad timestamps")
	}
	if m.cmdLog[0].durationLabel(time.Now()) == "" {
		t.Fatal("empty duration")
	}
}

func TestCmdLogInfiniteAndError(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 40
	m.showCmds = true
	id := m.beginCmd("kubectl logs --follow", true)
	if m.cmdLog[0].durationLabel(time.Now()) != "∞" {
		t.Fatalf("want ∞, got %q", m.cmdLog[0].durationLabel(time.Now()))
	}
	view := m.View()
	if !strings.Contains(view, "●") {
		t.Fatalf("missing running dot:\n%s", view)
	}
	if !strings.Contains(view, "∞") {
		t.Fatalf("missing infinity:\n%s", view)
	}
	m.finishCmd(id, errString("boom"))
	view = m.View()
	if !strings.Contains(view, "boom") {
		t.Fatalf("missing error:\n%s", view)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestCmdLogIDAtScreen(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 80
	m.height = 30
	m.showCmds = true
	id := m.beginCmd("kubectl get pods", false)

	bodyH, cmdsH := layoutHeights(m.height, true)
	y := 1 + bodyH + 2 // past top border + title → first cmd row
	got, ok := m.cmdLogIDAtScreen(10, y)
	if !ok || got != id {
		t.Fatalf("id=%d ok=%v (bodyH=%d cmdsH=%d y=%d)", got, ok, bodyH, cmdsH, y)
	}
}

func TestExecIntoContainerReturnsExecCmd(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30
	m.focus = paneRight
	m.rightView = viewPodDetail
	m.namespace = "ns"
	m.podName = "pod"
	m.podDetail = &kube.PodDetail{
		Name:       "pod",
		Containers: []kube.ContainerInfo{{Name: "app"}},
	}
	m.leftCursor = 1

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected exec cmd")
	}
	if len(m.cmdLog) == 0 || !strings.Contains(m.cmdLog[len(m.cmdLog)-1].Line, "exec") {
		t.Fatalf("cmdLog = %+v", m.cmdLog)
	}
}
