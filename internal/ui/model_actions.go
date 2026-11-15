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

func (model Model) ensureCredentials() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := model.selectedEnv()
	if !ok {
		model.err = "select an environment first"
		return model, nil
	}
	if env.Azure == nil {
		model.err = "no azure block for this environment"
		return model, nil
	}
	model.loading = true
	model.err = ""
	model.status = "ensuring credentials…"
	line := "az aks get-credentials + kubelogin (" + env.Name + ")"
	id := model.beginCmd(line, false)
	return model, tea.Batch(model.spinner.Tick, model.maybeCmdAnim(), func() tea.Msg {
		err := azure.EnsureCredentials(env)
		return ensureMsg{poolIdx: poolIdx, envIdx: envIdx, cmdID: id, err: err}
	})
}

func (model Model) azLogin() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := model.selectedEnv()
	if !ok {
		model.err = "select an environment first"
		return model, nil
	}
	if env.Azure == nil {
		model.err = "no azure block for this environment"
		return model, nil
	}
	if env.Azure.Subscription == "" {
		model.err = "azure.subscription is required for az login"
		return model, nil
	}
	supportsLoginSub := azure.DetectLoginSubscriptionSupport()
	model.loading = true
	model.err = ""
	line := "az login"
	if supportsLoginSub {
		model.status = "az login -s " + env.Azure.Subscription + "…"
		line = "az login -s " + env.Azure.Subscription
	} else {
		model.status = "az login…"
	}
	id := model.beginCmd(line, false)
	cmd := azure.PrepareLoginCmd(env.Azure.Subscription)
	return model, tea.Batch(model.maybeCmdAnim(), tea.ExecProcess(cmd, func(err error) tea.Msg {
		return loginMsg{poolIdx: poolIdx, envIdx: envIdx, cmdID: id, err: err}
	}))
}

func (model Model) openPodLogs() (tea.Model, tea.Cmd) {
	return model.startLogFollow()
}

func (model Model) execIntoContainer() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := model.selectedEnv()
	if !ok || model.podDetail == nil {
		return model, nil
	}
	container := model.selectedContainerName()
	if container == "" {
		model.err = "no container selected"
		return model, nil
	}
	args := kubectlExecArgs(env, model.namespace, model.podName, container)
	line := kubectlExecLine(env, model.namespace, model.podName, container)
	applog.Infof("starting interactive: kubectl %s", strings.Join(args, " "))
	model.loading = true
	model.err = ""
	model.status = fmt.Sprintf("exec %s / %s…", model.podName, container)
	id := model.beginCmd(line, false)
	// kubectl is a fixed binary; args are built from trusted config/selection.
	cmd := exec.Command("kubectl", args...) //nolint:gosec // G204: fixed kubectl binary, args from config
	return model, tea.Batch(model.maybeCmdAnim(), tea.ExecProcess(cmd, func(err error) tea.Msg {
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

func (model Model) refresh() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := model.selectedEnv()
	if !ok {
		return model, nil
	}
	if model.rightView == viewPodLogs {
		return model.startLogFollow()
	}
	model.loading = true
	model.err = ""
	model.status = ""
	model, cmd := model.quietRefreshFor(env, poolIdx, envIdx)
	return model, tea.Batch(model.spinner.Tick, cmd)
}

// quietRefreshFor re-fetches the current right view with silent=true for env.
func (model Model) quietRefreshFor(env config.Environment, poolIdx, envIdx int) (Model, tea.Cmd) {
	switch model.rightView {
	case viewPodDetail:
		ns, pod := model.namespace, model.podName
		return model.tracked(model.kubectlLinePodDetail(ns, pod), false, func() tea.Msg {
			msg := model.fetchPodDetail(env, poolIdx, envIdx, ns, pod).(podDetailMsg)
			msg.silent = true
			return msg
		})
	case viewPods:
		ns, dep := model.namespace, model.deployment
		return model.tracked(model.kubectlLinePods(ns, dep), false, func() tea.Msg {
			msg := model.fetchPods(env, poolIdx, envIdx, ns, dep).(podsMsg)
			msg.silent = true
			return msg
		})
	case viewWorkloads:
		ns := model.namespace
		return model.tracked(model.kubectlLineWorkloads(ns), false, func() tea.Msg {
			msg := model.fetchWorkloads(env, poolIdx, envIdx, ns).(workloadsMsg)
			msg.silent = true
			return msg
		})
	default:
		return model.tracked(model.kubectlLineNamespaces(), false, func() tea.Msg {
			msg := model.fetchNamespaces(env, poolIdx, envIdx).(namespacesMsg)
			msg.silent = true
			return msg
		})
	}
}

// quietRefresh re-fetches the current right view without a spinner/loading flash.
func (model Model) quietRefresh() (Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := model.selectedEnv()
	if !ok || !model.available[envKey(env)] {
		return model, nil
	}
	if !model.quietRefreshReady() {
		return model, nil
	}
	return model.quietRefreshFor(env, poolIdx, envIdx)
}

// quietRefreshReady reports whether the current view has enough context to refetch.
func (model Model) quietRefreshReady() bool {
	switch model.rightView {
	case viewPodDetail:
		return model.podName != ""
	case viewPods:
		return model.namespace != "" && model.deployment != ""
	case viewWorkloads:
		return model.namespace != ""
	case viewNamespaces:
		return true
	default:
		return false
	}
}
