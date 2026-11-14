package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/elmosgot/kube-commander/internal/config"
)

func (m Model) updateLeft(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.leftCursor > 0 {
			m.leftCursor--
		}
		return m, nil
	case keyDown, "j":
		if m.leftCursor < len(m.leftRows)-1 {
			m.leftCursor++
		}
		return m, nil
	case "enter", "right":
		return m.openLeftSelection()
	case "left", "h":
		return m.collapseLeftPool()
	}
	return m, nil
}

// openLeftSelection toggles a pool header or opens the selected environment.
func (m Model) openLeftSelection() (tea.Model, tea.Cmd) {
	if len(m.leftRows) == 0 {
		return m, nil
	}
	row := m.leftRows[m.leftCursor]
	if row.isPool {
		m.expanded[row.poolIdx] = !m.expanded[row.poolIdx]
		m.rebuildLeft()
		return m, nil
	}
	env := m.cfg.Pools[row.poolIdx].Environments[row.envIdx]
	m.stopLogFollow()
	m.resetForEnvOpen()
	m.focus = paneRight
	if env.Namespace != "" {
		m.rightView = viewWorkloads
		m.namespace = env.Namespace
		m, cmd := m.tracked(m.kubectlLineWorkloads(env.Namespace), false, func() tea.Msg {
			return m.fetchWorkloads(env, row.poolIdx, row.envIdx, env.Namespace)
		})
		return m, tea.Batch(m.spinner.Tick, cmd)
	}
	m.rightView = viewNamespaces
	m.namespace = ""
	m, cmd := m.tracked(m.kubectlLineNamespaces(), false, func() tea.Msg {
		return m.fetchNamespaces(env, row.poolIdx, row.envIdx)
	})
	return m, tea.Batch(m.spinner.Tick, cmd)
}

// resetForEnvOpen clears right-pane state before opening a freshly selected env.
func (m *Model) resetForEnvOpen() {
	m.loading = true
	m.autoRefreshBusy = false
	m.err = ""
	m.status = ""
	m.namespaces = nil
	m.workloads = nil
	m.pods = nil
	m.podDetail = nil
	m.deployment = ""
	m.podName = ""
	m.logText = ""
	m.logScroll = 0
	m.clearLogFilter()
	m.rightCursor = 0
}

// collapseLeftPool collapses the current pool and parks the cursor on its header.
func (m Model) collapseLeftPool() (tea.Model, tea.Cmd) {
	if len(m.leftRows) == 0 {
		return m, nil
	}
	row := m.leftRows[m.leftCursor]
	m.expanded[row.poolIdx] = false
	m.rebuildLeft()
	if !row.isPool {
		for i, r := range m.leftRows {
			if r.poolIdx == row.poolIdx && r.isPool {
				m.leftCursor = i
				break
			}
		}
	}
	return m, nil
}

func (m Model) updateRight(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.rightView == viewPodLogs && m.logFilterActive {
		return m.updateLogFilter(msg)
	}
	switch msg.String() {
	case "up", "k":
		return m.moveRight(-1)
	case keyDown, "j":
		return m.moveRight(1)
	case "pgup":
		return m.moveRight(-10)
	case "pgdown":
		return m.moveRight(10)
	case "enter", "right":
		return m.openRight()
	case "f", "/":
		if m.rightView == viewPodLogs {
			m.logFilterActive = true
			m.logFilter = ""
			m.logScroll = 0
			return m, nil
		}
	case "y":
		if m.rightView == viewPodLogs {
			if idx, ok := logLineAtVisualOffset(m.visualLogRows(), m.logScroll, 0); ok {
				m.copyLogLogicalLine(idx)
			}
			return m, nil
		}
	case "backspace", "left", "h", "esc":
		return m.backRight()
	}
	return m, nil
}

func (m Model) moveRight(delta int) (tea.Model, tea.Cmd) {
	switch m.rightView {
	case viewPodLogs:
		return m.scrollLogs(delta)
	case viewPodDetail:
		return m.moveContainer(delta)
	default:
		return m.moveCursor(delta)
	}
}

func (m Model) scrollLogs(delta int) (tea.Model, tea.Cmd) {
	maxScroll := m.maxLogScroll()
	m.logScroll += delta
	if m.logScroll < 0 {
		m.logScroll = 0
	}
	if m.logScroll > maxScroll {
		m.logScroll = maxScroll
	}
	m.logStickBottom = m.logScroll >= maxScroll
	return m, nil
}

func (m Model) moveContainer(delta int) (tea.Model, tea.Cmd) {
	if m.podDetail == nil || len(m.podDetail.Containers) == 0 {
		return m, nil
	}
	m.containerIdx += delta
	if m.containerIdx < 0 {
		m.containerIdx = 0
	}
	if m.containerIdx >= len(m.podDetail.Containers) {
		m.containerIdx = len(m.podDetail.Containers) - 1
	}
	return m, nil
}

func (m Model) moveCursor(delta int) (tea.Model, tea.Cmd) {
	max := m.rightLen() - 1
	if max < 0 {
		return m, nil
	}
	m.rightCursor += delta
	if m.rightCursor < 0 {
		m.rightCursor = 0
	}
	if m.rightCursor > max {
		m.rightCursor = max
	}
	return m, nil
}

func (m Model) openRight() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok {
		return m, nil
	}
	switch m.rightView {
	case viewNamespaces:
		return m.openNamespace(env, poolIdx, envIdx)
	case viewWorkloads:
		return m.openWorkload(env, poolIdx, envIdx)
	case viewPods:
		return m.openPod(env, poolIdx, envIdx)
	case viewPodDetail:
		return m.openPodLogs()
	}
	return m, nil
}

func (m Model) openNamespace(env config.Environment, poolIdx, envIdx int) (tea.Model, tea.Cmd) {
	if len(m.namespaces) == 0 {
		return m, nil
	}
	ns := m.namespaces[m.rightCursor].Name
	m.namespace = ns
	m.rightView = viewWorkloads
	m.workloads = nil
	m.rightCursor = 0
	m.loading = true
	m.err = ""
	m.status = ""
	m, cmd := m.tracked(m.kubectlLineWorkloads(ns), false, func() tea.Msg {
		return m.fetchWorkloads(env, poolIdx, envIdx, ns)
	})
	return m, tea.Batch(m.spinner.Tick, cmd)
}

func (m Model) openWorkload(env config.Environment, poolIdx, envIdx int) (tea.Model, tea.Cmd) {
	if len(m.workloads) == 0 {
		return m, nil
	}
	dep := m.workloads[m.rightCursor].Name
	m.deployment = dep
	m.rightView = viewPods
	m.pods = nil
	m.rightCursor = 0
	m.loading = true
	m.err = ""
	m.status = ""
	ns := m.namespace
	m, cmd := m.tracked(m.kubectlLinePods(ns, dep), false, func() tea.Msg {
		return m.fetchPods(env, poolIdx, envIdx, ns, dep)
	})
	return m, tea.Batch(m.spinner.Tick, cmd)
}

func (m Model) openPod(env config.Environment, poolIdx, envIdx int) (tea.Model, tea.Cmd) {
	if len(m.pods) == 0 {
		return m, nil
	}
	pod := m.pods[m.rightCursor].Name
	m.podName = pod
	m.rightView = viewPodDetail
	m.podDetail = nil
	m.containerIdx = 0
	m.loading = true
	m.err = ""
	m.status = ""
	ns := m.namespace
	m, cmd := m.tracked(m.kubectlLinePodDetail(ns, pod), false, func() tea.Msg {
		return m.fetchPodDetail(env, poolIdx, envIdx, ns, pod)
	})
	return m, tea.Batch(m.spinner.Tick, cmd)
}

func (m Model) backRight() (tea.Model, tea.Cmd) {
	switch m.rightView {
	case viewPodLogs:
		return m.backFromPodLogs()
	case viewPodDetail:
		return m.backToPods()
	case viewPods:
		return m.backToWorkloads()
	case viewWorkloads:
		return m.backToLeft()
	default:
		m.focus = paneLeft
		return m, nil
	}
}

func (m Model) backFromPodLogs() (tea.Model, tea.Cmd) {
	m.stopLogFollow()
	m.rightView = viewPodDetail
	m.logText = ""
	m.logScroll = 0
	m.logStickBottom = true
	m.clearLogFilter()
	m.err = ""
	m.status = ""
	return m, nil
}

func (m Model) backToPods() (tea.Model, tea.Cmd) {
	m.rightView = viewPods
	m.podDetail = nil
	m.podName = ""
	m.containerIdx = 0
	m.rightCursor = 0
	m.err = ""
	m.status = ""
	m.loading = true
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok {
		return m, nil
	}
	ns, dep := m.namespace, m.deployment
	m, cmd := m.tracked(m.kubectlLinePods(ns, dep), false, func() tea.Msg {
		return m.fetchPods(env, poolIdx, envIdx, ns, dep)
	})
	return m, tea.Batch(m.spinner.Tick, cmd)
}

func (m Model) backToWorkloads() (tea.Model, tea.Cmd) {
	m.rightView = viewWorkloads
	m.pods = nil
	m.deployment = ""
	m.rightCursor = 0
	m.err = ""
	m.status = ""
	m.loading = true
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok {
		return m, nil
	}
	ns := m.namespace
	m, cmd := m.tracked(m.kubectlLineWorkloads(ns), false, func() tea.Msg {
		return m.fetchWorkloads(env, poolIdx, envIdx, ns)
	})
	return m, tea.Batch(m.spinner.Tick, cmd)
}

// backToLeft always returns to the left pools pane (skip namespaces; often Forbidden).
func (m Model) backToLeft() (tea.Model, tea.Cmd) {
	m.workloads = nil
	m.namespace = ""
	m.rightCursor = 0
	m.err = ""
	m.status = ""
	m.loading = false
	m.focus = paneLeft
	return m, nil
}

func (m Model) rightLen() int {
	switch m.rightView {
	case viewWorkloads:
		return len(m.workloads)
	case viewPods:
		return len(m.pods)
	case viewPodDetail:
		if m.podDetail == nil {
			return 0
		}
		return len(m.podDetail.Containers)
	case viewPodLogs:
		return len(m.filteredLogLines())
	default:
		return len(m.namespaces)
	}
}
