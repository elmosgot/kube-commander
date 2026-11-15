package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elmosgot/kube-commander/internal/kube"
)

func TestKubectlLineBuilders(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.leftCursor = 1
	model.namespace = tApp
	model.deployment = tAPI
	model.podName = tAPI1
	model.rightView = viewWorkloads
	if got := model.kubectlLineWorkloads(tApp); !strings.Contains(got, "-n app get deployments") {
		test.Fatalf("workloads: %s", got)
	}
	model.rightView = viewPods
	if got := model.kubectlLinePods(tApp, tAPI); !strings.Contains(got, "get pods") {
		test.Fatalf("pods: %s", got)
	}
	model.podDetail = &kube.PodDetail{
		Name:       tAPI1,
		Containers: []kube.ContainerInfo{{Name: tApp}},
	}
	model.rightView = viewPodLogs
	if got := model.kubectlLogsFollowLine(); !strings.Contains(got, "logs api-1 --tail=200 --follow") {
		test.Fatalf("logs: %s", got)
	}
}

func TestToggleCommandsPanel(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 40
	model.leftCursor = 1
	model.beginCmd("kubectl get pods", false)

	if model.showCmds {
		test.Fatal("expected hidden by default")
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)
	if !model.showCmds {
		test.Fatal("expected showCmds after c")
	}
	view := model.View()
	if !strings.Contains(view, "Commands") {
		test.Fatalf("panel missing title:\n%s", view)
	}
	if strings.Contains(view, "\nView\n") || strings.Contains(view, "Shell") {
		test.Fatalf("old View/Shell sections still present:\n%s", view)
	}
	if !strings.Contains(view, "kubectl get pods") {
		test.Fatalf("panel missing live command:\n%s", view)
	}
}

func TestLayoutHeightsFitsTerminal(test *testing.T) {
	bodyH, cmdsH := layoutHeights(24, true)
	if bodyH+cmdsH+3 > 24 { // + chrome
		test.Fatalf("overflow: body=%d cmds=%d", bodyH, cmdsH)
	}
	if bodyH < 1 || cmdsH < 1 {
		test.Fatalf("expected positive body/cmds, got body=%d cmds=%d", bodyH, cmdsH)
	}

	bodyH, cmdsH = layoutHeights(10, true)
	if bodyH+cmdsH+3 > 10 {
		test.Fatalf("tight overflow: body=%d cmds=%d", bodyH, cmdsH)
	}
	if bodyH != 1 {
		test.Fatalf("expected bodyH=1 on tight terminal, got %d (cmds=%d)", bodyH, cmdsH)
	}

	bodyH, cmdsH = layoutHeights(40, false)
	if cmdsH != 0 || bodyH != 37 {
		test.Fatalf("hidden cmds: body=%d cmds=%d", bodyH, cmdsH)
	}
}

func TestCommandsPanelViewFitsHeight(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 80
	model.height = 16
	model.leftCursor = 1
	model.showCmds = true
	model.beginCmd("kubectl get namespaces", false)

	view := model.View()
	lines := strings.Split(view, "\n")
	if len(lines) > model.height+2 {
		test.Fatalf("view has %d lines for height %d:\n%s", len(lines), model.height, view)
	}
}

func TestCmdLogBeginFinish(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	id := model.beginCmd("kubectl get pods", false)
	if len(model.cmdLog) != 1 || !model.cmdLog[0].Running {
		test.Fatalf("entry = %+v", model.cmdLog)
	}
	time.Sleep(2 * time.Millisecond)
	model.finishCmd(id, nil)
	if model.cmdLog[0].Running {
		test.Fatal("still running")
	}
	if model.cmdLog[0].Ended.Before(model.cmdLog[0].Started) {
		test.Fatal("bad timestamps")
	}
	if model.cmdLog[0].durationLabel(time.Now()) == "" {
		test.Fatal("empty duration")
	}
}

func TestCmdLogInfiniteAndError(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 40
	model.showCmds = true
	cmdID := model.beginCmd("kubectl logs --follow", true)
	if model.cmdLog[0].durationLabel(time.Now()) != "∞" {
		test.Fatalf("want ∞, got %q", model.cmdLog[0].durationLabel(time.Now()))
	}
	view := model.View()
	if !strings.Contains(view, "●") {
		test.Fatalf("missing running dot:\n%s", view)
	}
	if !strings.Contains(view, "∞") {
		test.Fatalf("missing infinity:\n%s", view)
	}
	model.finishCmd(cmdID, errString("boom"))
	view = model.View()
	if !strings.Contains(view, "boom") {
		test.Fatalf("missing error:\n%s", view)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestCmdLogIDAtScreen(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 80
	model.height = 30
	model.showCmds = true
	id := model.beginCmd("kubectl get pods", false)

	bodyH, cmdsH := layoutHeights(model.height, true)
	y := 1 + bodyH + 2 // past top border + title → first cmd row
	got, ok := model.cmdLogIDAtScreen(10, y)
	if !ok || got != id {
		test.Fatalf("id=%d ok=%v (bodyH=%d cmdsH=%d y=%d)", got, ok, bodyH, cmdsH, y)
	}
}

func TestExecIntoContainerReturnsExecCmd(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30
	model.focus = paneRight
	model.rightView = viewPodDetail
	model.namespace = "ns"
	model.podName = "pod"
	model.podDetail = &kube.PodDetail{
		Name:       "pod",
		Containers: []kube.ContainerInfo{{Name: tApp}},
	}
	model.leftCursor = 1

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = updated.(Model)
	if cmd == nil {
		test.Fatal("expected exec cmd")
	}
	if len(model.cmdLog) == 0 || !strings.Contains(model.cmdLog[len(model.cmdLog)-1].Line, "exec") {
		test.Fatalf("cmdLog = %+v", model.cmdLog)
	}
}
