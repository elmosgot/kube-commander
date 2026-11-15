package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/elmosgot/kube-commander/internal/config"
)

func (model Model) updateLeft(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if model.leftCursor > 0 {
			model.leftCursor--
		}
		return model, nil
	case keyDown, "j":
		if model.leftCursor < len(model.leftRows)-1 {
			model.leftCursor++
		}
		return model, nil
	case "enter", "right":
		return model.openLeftSelection()
	case "left", "h":
		return model.collapseLeftPool()
	}
	return model, nil
}

// openLeftSelection toggles a pool header or opens the selected environment.
func (model Model) openLeftSelection() (tea.Model, tea.Cmd) {
	if len(model.leftRows) == 0 {
		return model, nil
	}
	row := model.leftRows[model.leftCursor]
	if row.isPool {
		model.expanded[row.poolIdx] = !model.expanded[row.poolIdx]
		model.rebuildLeft()
		return model, nil
	}
	env := model.cfg.Pools[row.poolIdx].Environments[row.envIdx]
	model.stopLogFollow()
	model.resetForEnvOpen()
	model.focus = paneRight
	if env.Namespace != "" {
		model.rightView = viewWorkloads
		model.namespace = env.Namespace
		m, cmd := model.tracked(model.kubectlLineWorkloads(env.Namespace), false, func() tea.Msg {
			return model.fetchWorkloads(env, row.poolIdx, row.envIdx, env.Namespace)
		})
		return m, tea.Batch(m.spinner.Tick, cmd)
	}
	model.rightView = viewNamespaces
	model.namespace = ""
	model, cmd := model.tracked(model.kubectlLineNamespaces(), false, func() tea.Msg {
		return model.fetchNamespaces(env, row.poolIdx, row.envIdx)
	})
	return model, tea.Batch(model.spinner.Tick, cmd)
}

// resetForEnvOpen clears right-pane state before opening a freshly selected env.
func (model *Model) resetForEnvOpen() {
	model.loading = true
	model.autoRefreshBusy = false
	model.err = ""
	model.status = ""
	model.namespaces = nil
	model.workloads = nil
	model.pods = nil
	model.podDetail = nil
	model.deployment = ""
	model.podName = ""
	model.logText = ""
	model.logScroll = 0
	model.clearLogFilter()
	model.rightCursor = 0
}

// collapseLeftPool collapses the current pool and parks the cursor on its header.
func (model Model) collapseLeftPool() (tea.Model, tea.Cmd) {
	if len(model.leftRows) == 0 {
		return model, nil
	}
	row := model.leftRows[model.leftCursor]
	model.expanded[row.poolIdx] = false
	model.rebuildLeft()
	if !row.isPool {
		for i, r := range model.leftRows {
			if r.poolIdx == row.poolIdx && r.isPool {
				model.leftCursor = i
				break
			}
		}
	}
	return model, nil
}

func (model Model) updateRight(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if model.rightView == viewPodLogs && model.logFilterActive {
		return model.updateLogFilter(msg)
	}
	switch msg.String() {
	case "up", "k":
		return model.moveRight(-1)
	case keyDown, "j":
		return model.moveRight(1)
	case "pgup":
		return model.moveRight(-10)
	case "pgdown":
		return model.moveRight(10)
	case "enter", "right":
		return model.openRight()
	case "f", "/":
		if model.rightView == viewPodLogs {
			model.logFilterActive = true
			model.logFilter = ""
			model.logScroll = 0
			return model, nil
		}
	case "y":
		if model.rightView == viewPodLogs {
			if idx, ok := logLineAtVisualOffset(model.visualLogRows(), model.logScroll, 0); ok {
				model.copyLogLogicalLine(idx)
			}
			return model, nil
		}
	case "backspace", "left", "h", "esc":
		return model.backRight()
	}
	return model, nil
}

func (model Model) moveRight(delta int) (tea.Model, tea.Cmd) {
	switch model.rightView {
	case viewPodLogs:
		return model.scrollLogs(delta)
	case viewPodDetail:
		return model.moveContainer(delta)
	default:
		return model.moveCursor(delta)
	}
}

func (model Model) scrollLogs(delta int) (tea.Model, tea.Cmd) {
	maxScroll := model.maxLogScroll()
	model.logScroll += delta
	if model.logScroll < 0 {
		model.logScroll = 0
	}
	if model.logScroll > maxScroll {
		model.logScroll = maxScroll
	}
	model.logStickBottom = model.logScroll >= maxScroll
	return model, nil
}

func (model Model) moveContainer(delta int) (tea.Model, tea.Cmd) {
	if model.podDetail == nil || len(model.podDetail.Containers) == 0 {
		return model, nil
	}
	model.containerIdx += delta
	if model.containerIdx < 0 {
		model.containerIdx = 0
	}
	if model.containerIdx >= len(model.podDetail.Containers) {
		model.containerIdx = len(model.podDetail.Containers) - 1
	}
	return model, nil
}

func (model Model) moveCursor(delta int) (tea.Model, tea.Cmd) {
	max := model.rightLen() - 1
	if max < 0 {
		return model, nil
	}
	model.rightCursor += delta
	if model.rightCursor < 0 {
		model.rightCursor = 0
	}
	if model.rightCursor > max {
		model.rightCursor = max
	}
	return model, nil
}

func (model Model) openRight() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := model.selectedEnv()
	if !ok {
		return model, nil
	}
	switch model.rightView {
	case viewNamespaces:
		return model.openNamespace(env, poolIdx, envIdx)
	case viewWorkloads:
		return model.openWorkload(env, poolIdx, envIdx)
	case viewPods:
		return model.openPod(env, poolIdx, envIdx)
	case viewPodDetail:
		return model.openPodLogs()
	}
	return model, nil
}

func (model Model) openNamespace(env config.Environment, poolIdx, envIdx int) (tea.Model, tea.Cmd) {
	if len(model.namespaces) == 0 {
		return model, nil
	}
	namespaceName := model.namespaces[model.rightCursor].Name
	model.namespace = namespaceName
	model.rightView = viewWorkloads
	model.workloads = nil
	model.rightCursor = 0
	model.loading = true
	model.err = ""
	model.status = ""
	model, cmd := model.tracked(model.kubectlLineWorkloads(namespaceName), false, func() tea.Msg {
		return model.fetchWorkloads(env, poolIdx, envIdx, namespaceName)
	})
	return model, tea.Batch(model.spinner.Tick, cmd)
}

func (model Model) openWorkload(env config.Environment, poolIdx, envIdx int) (tea.Model, tea.Cmd) {
	if len(model.workloads) == 0 {
		return model, nil
	}
	dep := model.workloads[model.rightCursor].Name
	model.deployment = dep
	model.rightView = viewPods
	model.pods = nil
	model.rightCursor = 0
	model.loading = true
	model.err = ""
	model.status = ""
	ns := model.namespace
	model, cmd := model.tracked(model.kubectlLinePods(ns, dep), false, func() tea.Msg {
		return model.fetchPods(env, poolIdx, envIdx, ns, dep)
	})
	return model, tea.Batch(model.spinner.Tick, cmd)
}

func (model Model) openPod(env config.Environment, poolIdx, envIdx int) (tea.Model, tea.Cmd) {
	if len(model.pods) == 0 {
		return model, nil
	}
	pod := model.pods[model.rightCursor].Name
	model.podName = pod
	model.rightView = viewPodDetail
	model.podDetail = nil
	model.containerIdx = 0
	model.loading = true
	model.err = ""
	model.status = ""
	ns := model.namespace
	model, cmd := model.tracked(model.kubectlLinePodDetail(ns, pod), false, func() tea.Msg {
		return model.fetchPodDetail(env, poolIdx, envIdx, ns, pod)
	})
	return model, tea.Batch(model.spinner.Tick, cmd)
}

func (model Model) backRight() (tea.Model, tea.Cmd) {
	switch model.rightView {
	case viewPodLogs:
		return model.backFromPodLogs()
	case viewPodDetail:
		return model.backToPods()
	case viewPods:
		return model.backToWorkloads()
	case viewWorkloads:
		return model.backToLeft()
	default:
		model.focus = paneLeft
		return model, nil
	}
}

func (model Model) backFromPodLogs() (tea.Model, tea.Cmd) {
	model.stopLogFollow()
	model.rightView = viewPodDetail
	model.logText = ""
	model.logScroll = 0
	model.logStickBottom = true
	model.clearLogFilter()
	model.err = ""
	model.status = ""
	return model, nil
}

func (model Model) backToPods() (tea.Model, tea.Cmd) {
	model.rightView = viewPods
	model.podDetail = nil
	model.podName = ""
	model.containerIdx = 0
	model.rightCursor = 0
	model.err = ""
	model.status = ""
	model.loading = true
	env, poolIdx, envIdx, ok := model.selectedEnv()
	if !ok {
		return model, nil
	}
	ns, dep := model.namespace, model.deployment
	model, cmd := model.tracked(model.kubectlLinePods(ns, dep), false, func() tea.Msg {
		return model.fetchPods(env, poolIdx, envIdx, ns, dep)
	})
	return model, tea.Batch(model.spinner.Tick, cmd)
}

func (model Model) backToWorkloads() (tea.Model, tea.Cmd) {
	model.rightView = viewWorkloads
	model.pods = nil
	model.deployment = ""
	model.rightCursor = 0
	model.err = ""
	model.status = ""
	model.loading = true
	env, poolIdx, envIdx, ok := model.selectedEnv()
	if !ok {
		return model, nil
	}
	ns := model.namespace
	model, cmd := model.tracked(model.kubectlLineWorkloads(ns), false, func() tea.Msg {
		return model.fetchWorkloads(env, poolIdx, envIdx, ns)
	})
	return model, tea.Batch(model.spinner.Tick, cmd)
}

// backToLeft always returns to the left pools pane (skip namespaces; often Forbidden).
func (model Model) backToLeft() (tea.Model, tea.Cmd) {
	model.workloads = nil
	model.namespace = ""
	model.rightCursor = 0
	model.err = ""
	model.status = ""
	model.loading = false
	model.focus = paneLeft
	return model, nil
}

func (model Model) rightLen() int {
	switch model.rightView {
	case viewWorkloads:
		return len(model.workloads)
	case viewPods:
		return len(model.pods)
	case viewPodDetail:
		if model.podDetail == nil {
			return 0
		}
		return len(model.podDetail.Containers)
	case viewPodLogs:
		return len(model.filteredLogLines())
	default:
		return len(model.namespaces)
	}
}
