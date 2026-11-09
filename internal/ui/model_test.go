package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elwin/kube-commander/internal/config"
	"github.com/elwin/kube-commander/internal/kube"
)

func testConfig() *config.Config {
	return &config.Config{
		Pools: []config.Pool{
			{
				Name: "pool-x",
				Environments: []config.Environment{
					{
						Name:      "testing",
						Context:   "pool-x-test",
						Namespace: "app",
						Azure: &config.AzureConfig{
							Subscription:  "00000000-0000-0000-0000-000000000002",
							ResourceGroup: "rg-pool-x-test",
							Cluster:       "aks-pool-x-test",
						},
					},
					{Name: "production", Context: "pool-x-prod", Namespace: "app"},
				},
			},
			{
				Name: "pool-y",
				Environments: []config.Environment{
					{Name: "develop", Context: "pool-y-dev", Namespace: "app"},
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
	for _, want := range []string{"kube-commander", "Pools", "pool-x", "testing", "pool-y", "namespace=app", "kubeconfig=", "a az login", "g logs"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
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
	if !ok || env.Name != "testing" || env.Context != "pool-x-test" {
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
	if m.status == "" || !strings.Contains(m.status, "az login -s") {
		t.Fatalf("status = %q", m.status)
	}
	if m.err != "" {
		t.Fatalf("unexpected err %q", m.err)
	}
}

func TestUnavailableHintMentionsAzLogin(t *testing.T) {
	env := config.Environment{
		Context: "pool-x-test",
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
	m.namespace = "app"

	updated, _ := m.Update(podsMsg{
		poolIdx:    0,
		envIdx:     0,
		namespace:  "app",
		deployment: "my-api",
		items: []kube.PodInfo{
			{Name: "my-api-abc", Ready: "1/1", Restarts: 0, Status: "Running", Age: "3d"},
		},
	})
	m = updated.(Model)
	if m.rightView != viewPods {
		t.Fatalf("rightView = %v", m.rightView)
	}
	if m.deployment != "my-api" || len(m.pods) != 1 {
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
	m.deployment = "my-api"
	m.rightView = viewPods

	updated, _ := m.Update(podDetailMsg{
		poolIdx:   0,
		envIdx:    0,
		namespace: "ns",
		pod:       "my-api-abc",
		detail: &kube.PodDetail{
			Name:  "my-api-abc",
			Phase: "Running",
			Ready: "1/1",
			Node:  "node-1",
			PodIP: "10.0.0.1",
			Age:   "2d",
			Containers: []kube.ContainerInfo{
				{
					Name: "app", Image: "img:1", Ready: true, State: "Running",
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
	for _, want := range []string{"Pod · my-api-abc", "phase=Running", "app", "cpu 100m/500m", "mem 128Mi/512Mi", "container=app"} {
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
	m.namespace = "ns"
	m.deployment = "dep"
	m.podName = "pod"
	m.podDetail = &kube.PodDetail{
		Name:       "pod",
		Containers: []kube.ContainerInfo{{Name: "app"}},
	}
	m.rightView = viewPodDetail

	updated, _ := m.Update(podLogsMsg{
		poolIdx: 0, envIdx: 0, namespace: "ns", pod: "pod", container: "app",
		text: "line0\nline1\nline2\nline3\nline4",
	})
	m = updated.(Model)
	if m.rightView != viewPodLogs {
		t.Fatalf("rightView = %v", m.rightView)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.logScroll != 1 {
		t.Fatalf("logScroll = %d", m.logScroll)
	}
}
