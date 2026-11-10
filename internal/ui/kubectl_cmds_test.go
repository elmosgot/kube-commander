package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestKubectlRecipesNamespaces(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.leftCursor = 1 // first env under pool-x
	m.rightView = viewNamespaces
	lines := m.kubectlRecipes()
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"kubectl --context pool-x-test",
		"--kubeconfig",
		"get namespaces",
		"get deployments -A",
		"get pods -A",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
}

func TestKubectlRecipesWorkloadsPodsLogs(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.leftCursor = 1
	m.namespace = "app"
	m.deployment = "api"
	m.podName = "api-1"
	m.podDetail = nil

	m.rightView = viewWorkloads
	if got := strings.Join(m.kubectlRecipes(), "\n"); !strings.Contains(got, "-n app get deployments") {
		t.Fatalf("workloads: %s", got)
	}

	m.rightView = viewPods
	pods := strings.Join(m.kubectlRecipes(), "\n")
	if !strings.Contains(pods, "get deployment api") || !strings.Contains(pods, "get pods") {
		t.Fatalf("pods: %s", pods)
	}

	m.rightView = viewPodDetail
	if got := strings.Join(m.kubectlRecipes(), "\n"); !strings.Contains(got, "get pod api-1 -o wide") {
		t.Fatalf("detail: %s", got)
	}

	m.rightView = viewPodLogs
	if got := strings.Join(m.kubectlRecipes(), "\n"); !strings.Contains(got, "logs api-1 --tail=200") {
		t.Fatalf("logs: %s", got)
	}
}

func TestToggleCommandsPanel(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 40
	m.leftCursor = 1

	if m.showCmds {
		t.Fatal("expected hidden by default")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updated.(Model)
	if !m.showCmds {
		t.Fatal("expected showCmds after c")
	}
	view := m.View()
	if !strings.Contains(view, "View") || !strings.Contains(view, "Shell") {
		t.Fatalf("panel missing sections:\n%s", view)
	}
	if !strings.Contains(view, "get namespaces") {
		t.Fatalf("panel missing kubectl recipe:\n%s", view)
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

	// Tight terminal: must shrink cmds instead of flooring body to 3.
	bodyH, cmdsH = layoutHeights(10, true)
	if bodyH+cmdsH+3 > 10 {
		t.Fatalf("tight overflow: body=%d cmds=%d", bodyH, cmdsH)
	}
	if bodyH != 1 {
		// avail=7, cmds capped to 6 → body=1
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
	m.height = 16 // would overflow under old bodyH=3 floor + cmdsPanelHeight
	m.leftCursor = 1
	m.showCmds = true

	view := m.View()
	lines := strings.Split(view, "\n")
	// lipgloss may leave a trailing empty line; allow a small margin but not full overflow
	if len(lines) > m.height+2 {
		t.Fatalf("view has %d lines for height %d:\n%s", len(lines), m.height, view)
	}
}
