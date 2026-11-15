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

func TestModelRendersPools(test *testing.T) {
	model := New(testConfig(), "configs/example.yaml")
	model.width = 120
	model.height = 40

	view := model.View()
	for _, want := range []string{
		"kube-commander", "Pools", "pool-x", tTesting, "pool-y",
		"namespace=app", "kubeconfig=", helpAzLogin, helpEnsure, "c commands",
	} {
		if !strings.Contains(view, want) {
			test.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	for _, notWant := range []string{helpLogs, helpFilter, helpShell} {
		if strings.Contains(view, notWant) {
			test.Fatalf("left-pane view unexpectedly has %q:\n%s", notWant, view)
		}
	}
	if !strings.Contains(view, config.ResolveKubeconfig(testConfig().Pools[0].Environments[0])) {
		test.Fatalf("view missing resolved kubeconfig path:\n%s", view)
	}
}

func TestLeftNavigationSelectsEnvironment(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30

	env, _, _, ok := model.selectedEnv()
	if !ok || env.Name != tTesting || env.Context != tPoolXTest {
		test.Fatalf("initial env = %+v ok=%v", env, ok)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	env, _, _, ok = model.selectedEnv()
	if !ok || env.Name != "production" {
		test.Fatalf("after down env = %+v ok=%v", env, ok)
	}
}

func TestEnterOpensDefaultNamespace(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.rightView != viewWorkloads {
		test.Fatalf("rightView = %v, want workloads", model.rightView)
	}
	if model.namespace != "app" {
		test.Fatalf("namespace = %q", model.namespace)
	}
	if !model.loading {
		test.Fatal("expected loading")
	}
	if cmd == nil {
		test.Fatal("expected fetch command")
	}
}

func TestEnsureWithoutAzureShowsError(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model = updated.(Model)
	if cmd != nil {
		test.Fatal("expected no ensure command without azure")
	}
	if !strings.Contains(model.err, "no azure block") {
		test.Fatalf("err = %q", model.err)
	}
}

func TestAzLoginWithoutSubscriptionShowsError(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(Model)
	if cmd != nil {
		test.Fatal("expected no login command without azure")
	}
	if !strings.Contains(model.err, "no azure block") {
		test.Fatalf("err = %q", model.err)
	}
}

func TestAzLoginWithSubscriptionReturnsExecCmd(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(Model)
	if cmd == nil {
		test.Fatal("expected exec command for az login")
	}
	if model.status == "" || !strings.Contains(model.status, "az login") {
		test.Fatalf("status = %q", model.status)
	}
	if model.err != "" {
		test.Fatalf("unexpected err %q", model.err)
	}
}

func TestUnavailableHintMentionsAzLogin(test *testing.T) {
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
		test.Fatalf("hint = %q", hint)
	}
}

func TestTogglePoolCollapse(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 30

	for model.leftCursor > 0 {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
		model = updated.(Model)
	}
	if !model.leftRows[model.leftCursor].isPool {
		test.Fatal("expected pool header")
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if model.expanded[0] {
		test.Fatal("expected pool collapsed after enter")
	}
	for _, row := range model.leftRows {
		if row.poolIdx == 0 && !row.isPool {
			test.Fatal("collapsed pool still shows environments")
		}
	}
}

func TestPodsMsgOpensPodsView(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 120
	model.height = 40
	model.focus = paneRight
	model.namespace = tApp

	updated, _ := model.Update(podsMsg{
		poolIdx:    0,
		envIdx:     0,
		namespace:  tApp,
		deployment: tMyAPI,
		items: []kube.PodInfo{
			{Name: tMyAPIAbc, Ready: tReady11, Restarts: 0, Status: tRunning, Age: "3d"},
		},
	})
	model = updated.(Model)
	if model.rightView != viewPods {
		test.Fatalf("rightView = %v", model.rightView)
	}
	if model.deployment != tMyAPI || len(model.pods) != 1 {
		test.Fatalf("deployment=%q pods=%d", model.deployment, len(model.pods))
	}
	view := model.View()
	if !strings.Contains(view, "Pods · my-api") || !strings.Contains(view, "my-api-abc") {
		test.Fatalf("view:\n%s", view)
	}
	if !strings.Contains(view, "deploy=my-api") {
		test.Fatalf("footer missing deploy:\n%s", view)
	}
}

func TestPodDetailAndBackspace(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 140
	model.height = 40
	model.focus = paneRight
	model.namespace = "ns"
	model.deployment = tMyAPI
	model.rightView = viewPods

	updated, _ := model.Update(podDetailMsg{
		poolIdx:   0,
		envIdx:    0,
		namespace: "ns",
		pod:       tMyAPIAbc,
		detail: &kube.PodDetail{
			Name:  tMyAPIAbc,
			Phase: tRunning,
			Ready: tReady11,
			Node:  "node-1",
			PodIP: "10.0.0.1",
			Age:   "2d",
			Containers: []kube.ContainerInfo{
				{
					Name: tApp, Image: "img:1", Ready: true, State: tRunning,
					CPURequest: "100m", CPULimit: "500m", MemRequest: "128Mi", MemLimit: "512Mi",
				},
			},
		},
	})
	model = updated.(Model)
	if model.rightView != viewPodDetail {
		test.Fatalf("rightView = %v", model.rightView)
	}
	view := model.View()
	for _, want := range []string{
		"Pod · my-api-abc", "phase=Running", tApp,
		"cpu 100m/500m", "mem 128Mi/512Mi", "container=app",
	} {
		if !strings.Contains(view, want) {
			test.Fatalf("missing %q in:\n%s", want, view)
		}
	}

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(Model)
	if model.rightView != viewPods {
		test.Fatalf("after backspace rightView = %v", model.rightView)
	}
	if cmd == nil {
		test.Fatal("expected reload pods command")
	}
}

func TestPodLogsScroll(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 20
	model.focus = paneRight
	model.rightView = viewPodLogs
	// height 20 → bodyH 17 → visible 13; 20 lines → maxScroll = 7
	var buf strings.Builder
	for i := 0; i < 20; i++ {
		if i > 0 {
			buf.WriteByte('\n')
		}
		fmt.Fprintf(&buf, "line%d", i)
	}
	model.logText = buf.String()
	model.scrollLogsToBottom()
	if model.logScroll != 7 {
		test.Fatalf("bottom scroll = %d, want 7", model.logScroll)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	if model.logScroll != 6 {
		test.Fatalf("logScroll = %d", model.logScroll)
	}
	if model.logStickBottom {
		test.Fatal("expected stick-to-bottom cleared after scrolling up")
	}
}

func TestScrollLogsToBottomShowsViewport(test *testing.T) {
	model := New(testConfig(), "test.yaml")
	model.width = 100
	model.height = 20
	model.rightView = viewPodLogs
	model.logStickBottom = true
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("L%d", i)
	}
	model.logText = strings.Join(lines, "\n")
	model.scrollLogsToBottom()

	var b strings.Builder
	model.renderPodLogs(&b, 80, 17)
	out := b.String()
	if !strings.Contains(out, "L29") {
		test.Fatalf("missing last line:\n%s", out)
	}
	if !strings.Contains(out, "L17") {
		test.Fatalf("expected full viewport near bottom, got:\n%s", out)
	}
	if strings.Contains(out, "L0\n") && !strings.Contains(out, "L17") {
		test.Fatal("viewport should not be only the last line")
	}
}
