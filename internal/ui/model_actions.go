package ui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elmosgot/kube-commander/internal/applog"
	"github.com/elmosgot/kube-commander/internal/azure"
	"github.com/elmosgot/kube-commander/internal/config"
)

func (m Model) ensureCredentials() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok {
		m.err = "select an environment first"
		return m, nil
	}
	if env.Azure == nil {
		m.err = "no azure block for this environment"
		return m, nil
	}
	m.loading = true
	m.err = ""
	m.status = "ensuring credentials…"
	line := "az aks get-credentials + kubelogin (" + env.Name + ")"
	id := m.beginCmd(line, false)
	return m, tea.Batch(m.spinner.Tick, m.maybeCmdAnim(), func() tea.Msg {
		err := azure.EnsureCredentials(env)
		return ensureMsg{poolIdx: poolIdx, envIdx: envIdx, cmdID: id, err: err}
	})
}

func (m Model) azLogin() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok {
		m.err = "select an environment first"
		return m, nil
	}
	if env.Azure == nil {
		m.err = "no azure block for this environment"
		return m, nil
	}
	if env.Azure.Subscription == "" {
		m.err = "azure.subscription is required for az login"
		return m, nil
	}
	supportsLoginSub := azure.DetectLoginSubscriptionSupport()
	m.loading = true
	m.err = ""
	line := "az login"
	if supportsLoginSub {
		m.status = "az login -s " + env.Azure.Subscription + "…"
		line = "az login -s " + env.Azure.Subscription
	} else {
		m.status = "az login…"
	}
	id := m.beginCmd(line, false)
	cmd := azure.PrepareLoginCmd(env.Azure.Subscription)
	return m, tea.Batch(m.maybeCmdAnim(), tea.ExecProcess(cmd, func(err error) tea.Msg {
		return loginMsg{poolIdx: poolIdx, envIdx: envIdx, cmdID: id, err: err}
	}))
}

func (m Model) openPodLogs() (tea.Model, tea.Cmd) {
	return m.startLogFollow()
}

func (m Model) execIntoContainer() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok || m.podDetail == nil {
		return m, nil
	}
	container := m.selectedContainerName()
	if container == "" {
		m.err = "no container selected"
		return m, nil
	}
	args := kubectlExecArgs(env, m.namespace, m.podName, container)
	line := kubectlExecLine(env, m.namespace, m.podName, container)
	applog.Infof("starting interactive: kubectl %s", strings.Join(args, " "))
	m.loading = true
	m.err = ""
	m.status = fmt.Sprintf("exec %s / %s…", m.podName, container)
	id := m.beginCmd(line, false)
	// kubectl is a fixed binary; args are built from trusted config/selection.
	cmd := exec.Command("kubectl", args...) //nolint:gosec // G204: fixed kubectl binary, args from config
	return m, tea.Batch(m.maybeCmdAnim(), tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execMsg{poolIdx: poolIdx, envIdx: envIdx, cmdID: id, err: err}
	}))
}

// kubectlExecArgs builds kubectl exec -it … -- sh -c 'bash || sh'.
func kubectlExecArgs(env config.Environment, namespace, pod, container string) []string {
	return []string{
		"exec", "-it",
		"--context", env.Context,
		"--kubeconfig", config.ResolveKubeconfig(env),
		"-n", namespace,
		pod,
		"-c", container,
		"--",
		"sh", "-c", "command -v bash >/dev/null 2>&1 && exec bash || exec sh",
	}
}

func (m Model) refresh() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok {
		return m, nil
	}
	if m.rightView == viewPodLogs {
		return m.startLogFollow()
	}
	m.loading = true
	m.err = ""
	m.status = ""
	m, cmd := m.quietRefreshFor(env, poolIdx, envIdx)
	return m, tea.Batch(m.spinner.Tick, cmd)
}

// quietRefreshFor re-fetches the current right view with silent=true for env.
func (m Model) quietRefreshFor(env config.Environment, poolIdx, envIdx int) (Model, tea.Cmd) {
	switch m.rightView {
	case viewPodDetail:
		ns, pod := m.namespace, m.podName
		return m.tracked(m.kubectlLinePodDetail(ns, pod), false, func() tea.Msg {
			msg := m.fetchPodDetail(env, poolIdx, envIdx, ns, pod).(podDetailMsg)
			msg.silent = true
			return msg
		})
	case viewPods:
		ns, dep := m.namespace, m.deployment
		return m.tracked(m.kubectlLinePods(ns, dep), false, func() tea.Msg {
			msg := m.fetchPods(env, poolIdx, envIdx, ns, dep).(podsMsg)
			msg.silent = true
			return msg
		})
	case viewWorkloads:
		ns := m.namespace
		return m.tracked(m.kubectlLineWorkloads(ns), false, func() tea.Msg {
			msg := m.fetchWorkloads(env, poolIdx, envIdx, ns).(workloadsMsg)
			msg.silent = true
			return msg
		})
	default:
		return m.tracked(m.kubectlLineNamespaces(), false, func() tea.Msg {
			msg := m.fetchNamespaces(env, poolIdx, envIdx).(namespacesMsg)
			msg.silent = true
			return msg
		})
	}
}

// quietRefresh re-fetches the current right view without a spinner/loading flash.
func (m Model) quietRefresh() (Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok || !m.available[envKey(env)] {
		return m, nil
	}
	if !m.quietRefreshReady() {
		return m, nil
	}
	return m.quietRefreshFor(env, poolIdx, envIdx)
}

// quietRefreshReady reports whether the current view has enough context to refetch.
func (m Model) quietRefreshReady() bool {
	switch m.rightView {
	case viewPodDetail:
		return m.podName != ""
	case viewPods:
		return m.namespace != "" && m.deployment != ""
	case viewWorkloads:
		return m.namespace != ""
	case viewNamespaces:
		return true
	default:
		return false
	}
}
