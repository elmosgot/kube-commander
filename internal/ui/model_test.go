package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elmosgot/kube-commander/internal/config"
	"github.com/elmosgot/kube-commander/internal/kube"
)

func testConfig() *config.Config {
	return &config.Config{
		Pools: []config.Pool{
			{
				Name: "pool-x",
				Environments: []config.Environment{
					{
						Name:      tTesting,
						Context:   tPoolXTest,
						Namespace: tApp,
						Azure: &config.AzureConfig{
							Subscription:  "00000000-0000-0000-0000-000000000002",
							ResourceGroup: "rg-pool-x-test",
							Cluster:       "aks-pool-x-test",
						},
					},
					{Name: "production", Context: "pool-x-prod", Namespace: tApp},
				},
			},
			{
				Name: "pool-y",
				Environments: []config.Environment{
					{Name: "develop", Context: "pool-y-dev", Namespace: tApp},
				},
			},
		},
	}
}

func TestModelRendersPools(t *testing.T) {
	m := New(testConfig(), "configs/example.yaml")
	m.width = 120
	m.height = 40

	view := m.View()
	for _, want := range []string{
		"kube-commander", "Pools", "pool-x", tTesting, "pool-y",
		"namespace=app", "kubeconfig=", helpAzLogin, "l ensure", "c commands",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	for _, notWant := range []string{helpLogs, helpFilter, "s shell"} {
		if strings.Contains(view, notWant) {
			t.Fatalf("left-pane view unexpectedly has %q:\n%s", notWant, view)
		}
	}
	if !strings.Contains(view, config.ResolveKubeconfig(testConfig().Pools[0].Environments[0])) {
		t.Fatalf("view missing resolved kubeconfig path:\n%s", view)
	}
}

func TestLeftNavigationSelectsEnvironment(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30

	env, _, _, ok := m.selectedEnv()
	if !ok || env.Name != tTesting || env.Context != tPoolXTest {
		t.Fatalf("initial env = %+v ok=%v", env, ok)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	env, _, _, ok = m.selectedEnv()
	if !ok || env.Name != "production" {
		t.Fatalf("after down env = %+v ok=%v", env, ok)
	}
}

func TestEnterOpensDefaultNamespace(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.rightView != viewWorkloads {
		t.Fatalf("rightView = %v, want workloads", m.rightView)
	}
	if m.namespace != "app" {
		t.Fatalf("namespace = %q", m.namespace)
	}
	if !m.loading {
		t.Fatal("expected loading")
	}
	if cmd == nil {
		t.Fatal("expected fetch command")
	}
}

func TestEnsureWithoutAzureShowsError(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no ensure command without azure")
	}
	if !strings.Contains(m.err, "no azure block") {
		t.Fatalf("err = %q", m.err)
	}
}

func TestAzLoginWithoutSubscriptionShowsError(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("expected no login command without azure")
	}
	if !strings.Contains(m.err, "no azure block") {
		t.Fatalf("err = %q", m.err)
	}
}

func TestAzLoginWithSubscriptionReturnsExecCmd(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("expected exec command for az login")
	}
	if m.status == "" || !strings.Contains(m.status, "az login") {
		t.Fatalf("status = %q", m.status)
	}
	if m.err != "" {
		t.Fatalf("unexpected err %q", m.err)
	}
}

func TestUnavailableHintMentionsAzLogin(t *testing.T) {
	env := config.Environment{
		Context: tPoolXTest,
		Azure: &config.AzureConfig{
			Subscription:  "00000000-0000-0000-0000-000000000002",
			ResourceGroup: "rg",
			Cluster:       "c",
		},
	}
	hint := unavailableHint(env)
	if !strings.Contains(hint, "press a to az login") {
		t.Fatalf("hint = %q", hint)
	}
}

func TestTogglePoolCollapse(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 30

	for m.leftCursor > 0 {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m = updated.(Model)
	}
	if !m.leftRows[m.leftCursor].isPool {
		t.Fatal("expected pool header")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.expanded[0] {
		t.Fatal("expected pool collapsed after enter")
	}
	for _, row := range m.leftRows {
		if row.poolIdx == 0 && !row.isPool {
			t.Fatal("collapsed pool still shows environments")
		}
	}
}

func TestPodsMsgOpensPodsView(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 120
	m.height = 40
	m.focus = paneRight
	m.namespace = tApp

	updated, _ := m.Update(podsMsg{
		poolIdx:    0,
		envIdx:     0,
		namespace:  tApp,
		deployment: tMyAPI,
		items: []kube.PodInfo{
			{Name: "my-api-abc", Ready: tReady11, Restarts: 0, Status: "Running", Age: "3d"},
		},
	})
	m = updated.(Model)
	if m.rightView != viewPods {
		t.Fatalf("rightView = %v", m.rightView)
	}
	if m.deployment != tMyAPI || len(m.pods) != 1 {
		t.Fatalf("deployment=%q pods=%d", m.deployment, len(m.pods))
	}
	view := m.View()
	if !strings.Contains(view, "Pods · my-api") || !strings.Contains(view, "my-api-abc") {
		t.Fatalf("view:\n%s", view)
	}
	if !strings.Contains(view, "deploy=my-api") {
		t.Fatalf("footer missing deploy:\n%s", view)
	}
}

func TestPodDetailAndBackspace(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 140
	m.height = 40
	m.focus = paneRight
	m.namespace = "ns"
	m.deployment = tMyAPI
	m.rightView = viewPods

	updated, _ := m.Update(podDetailMsg{
		poolIdx:   0,
		envIdx:    0,
		namespace: "ns",
		pod:       "my-api-abc",
		detail: &kube.PodDetail{
			Name:  "my-api-abc",
			Phase: "Running",
			Ready: tReady11,
			Node:  "node-1",
			PodIP: "10.0.0.1",
			Age:   "2d",
			Containers: []kube.ContainerInfo{
				{
					Name: tApp, Image: "img:1", Ready: true, State: "Running",
					CPURequest: "100m", CPULimit: "500m", MemRequest: "128Mi", MemLimit: "512Mi",
				},
			},
		},
	})
	m = updated.(Model)
	if m.rightView != viewPodDetail {
		t.Fatalf("rightView = %v", m.rightView)
	}
	view := m.View()
	for _, want := range []string{
		"Pod · my-api-abc", "phase=Running", tApp,
		"cpu 100m/500m", "mem 128Mi/512Mi", "container=app",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in:\n%s", want, view)
		}
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(Model)
	if m.rightView != viewPods {
		t.Fatalf("after backspace rightView = %v", m.rightView)
	}
	if cmd == nil {
		t.Fatal("expected reload pods command")
	}
}

func TestPodLogsScroll(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 20
	m.focus = paneRight
	m.rightView = viewPodLogs
	// height 20 → bodyH 17 → visible 13; 20 lines → maxScroll = 7
	var b strings.Builder
	for i := 0; i < 20; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "line%d", i)
	}
	m.logText = b.String()
	m.scrollLogsToBottom()
	if m.logScroll != 7 {
		t.Fatalf("bottom scroll = %d, want 7", m.logScroll)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.logScroll != 6 {
		t.Fatalf("logScroll = %d", m.logScroll)
	}
	if m.logStickBottom {
		t.Fatal("expected stick-to-bottom cleared after scrolling up")
	}
}

func TestScrollLogsToBottomShowsViewport(t *testing.T) {
	m := New(testConfig(), "test.yaml")
	m.width = 100
	m.height = 20
	m.rightView = viewPodLogs
	m.logStickBottom = true
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("L%d", i)
	}
	m.logText = strings.Join(lines, "\n")
	m.scrollLogsToBottom()

	var b strings.Builder
	m.renderPodLogs(&b, 80, 17)
	out := b.String()
	if !strings.Contains(out, "L29") {
		t.Fatalf("missing last line:\n%s", out)
	}
	if !strings.Contains(out, "L17") {
		t.Fatalf("expected full viewport near bottom, got:\n%s", out)
	}
	if strings.Contains(out, "L0\n") && !strings.Contains(out, "L17") {
		t.Fatal("viewport should not be only the last line")
	}
}
