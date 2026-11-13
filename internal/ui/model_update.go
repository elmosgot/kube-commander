package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/elmosgot/kube-commander/internal/applog"
	"github.com/elmosgot/kube-commander/internal/azure"
	"github.com/elmosgot/kube-commander/internal/kube"
)

// Update handles keyboard and async messages by dispatching to per-message handlers.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onWindowSize(msg)
	case tea.MouseMsg:
		return m.onMouse(msg)
	case spinner.TickMsg:
		return m.onSpinnerTick(msg)
	case cmdAnimMsg:
		return m.onCmdAnim()
	case cmdDoneMsg:
		return m.onCmdDone(msg)
	case initFetchMsg:
		return m.openSelectedEnvTracked()
	case autoRefreshMsg:
		return m.onAutoRefresh()
	case namespacesMsg:
		return m.onNamespacesMsg(msg)
	case workloadsMsg:
		return m.onWorkloadsMsg(msg)
	case podsMsg:
		return m.onPodsMsg(msg)
	case podDetailMsg:
		return m.onPodDetailMsg(msg)
	case logChunkMsg:
		return m.onLogChunk(msg)
	case logFollowDoneMsg:
		return m.onLogFollowDone(msg)
	case ensureMsg:
		return m.onEnsureMsg(msg)
	case loginMsg:
		return m.onLoginMsg(msg)
	case execMsg:
		return m.onExecMsg(msg)
	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m Model) onWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	return m, nil
}

func (m Model) onMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft || !msg.Ctrl {
		return m, nil
	}
	if m.showCmds {
		if handled, mm := m.ctrlClickCmdLog(msg.X, msg.Y); handled {
			return mm, nil
		}
	}
	if m.rightView == viewPodLogs {
		if idx, ok := m.logLogicalIdxAtScreen(msg.X, msg.Y); ok {
			m.copyLogLogicalLine(idx)
		}
	}
	return m, nil
}

// ctrlClickCmdLog copies the clicked command-log line; handled is false when the
// click did not land on a command entry.
func (m Model) ctrlClickCmdLog(x, y int) (handled bool, out Model) {
	id, ok := m.cmdLogIDAtScreen(x, y)
	if !ok {
		return false, m
	}
	e, found := cmdEntryByID(m.cmdLog, id)
	if !found {
		return false, m
	}
	if err := copyToClipboard(e.Line); err != nil {
		m.status = "clipboard failed: " + err.Error()
	} else {
		m.status = "copied"
		m.err = ""
	}
	return true, m
}

func (m Model) onSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, tea.Batch(cmd, m.maybeCmdAnim())
}

func (m Model) onCmdAnim() (tea.Model, tea.Cmd) {
	m.cmdAnimFrame++
	return m, m.maybeCmdAnim()
}

func (m Model) onCmdDone(msg cmdDoneMsg) (tea.Model, tea.Cmd) {
	m.finishCmd(msg.id, msg.err)
	inner, cmd := m.Update(msg.inner)
	m2 := inner.(Model)
	return m2, tea.Batch(cmd, m2.maybeCmdAnim())
}

func (m Model) onAutoRefresh() (tea.Model, tea.Cmd) {
	next := scheduleAutoRefresh()
	if m.rightView == viewPodLogs || m.loading || m.autoRefreshBusy {
		return m, next
	}
	var cmd tea.Cmd
	m, cmd = m.quietRefresh()
	if cmd == nil {
		return m, next
	}
	m.autoRefreshBusy = true
	return m, tea.Batch(next, cmd)
}

// staleFor reports whether an async message targets a different environment than
// the current selection, and clears the busy flag for silent (auto-refresh) msgs.
func (m *Model) staleFor(poolIdx, envIdx int, silent bool) bool {
	_, curPool, curEnv, ok := m.selectedEnv()
	if ok && curPool == poolIdx && curEnv == envIdx {
		return false
	}
	if silent {
		m.autoRefreshBusy = false
	}
	return true
}

func (m Model) onNamespacesMsg(msg namespacesMsg) (tea.Model, tea.Cmd) {
	if m.staleFor(msg.poolIdx, msg.envIdx, msg.silent) {
		return m, nil
	}
	if msg.silent {
		m.autoRefreshBusy = false
		if m.rightView != viewNamespaces {
			return m, nil
		}
	}
	m.loading = false
	if !msg.silent {
		m.deployment = ""
		m.podName = ""
		m.podDetail = nil
		m.workloads = nil
		m.pods = nil
		m.rightCursor = 0
	}
	if msg.err != nil {
		return m.namespacesError(msg.err)
	}
	m.rightView = viewNamespaces
	m.namespace = ""
	m.err = ""
	m.namespaces = msg.items
	m.rightCursor = clampIndex(m.rightCursor, len(m.namespaces))
	return m, nil
}

func (m Model) namespacesError(err error) (tea.Model, tea.Cmd) {
	m.namespaces = nil
	if kube.IsForbidden(err) {
		m.err = "cannot list namespaces (RBAC). Set environments[].namespace in config, then reopen the environment."
		m.focus = paneLeft
		m.namespace = ""
		m.status = ""
		return m, nil
	}
	m.rightView = viewNamespaces
	m.namespace = ""
	m.err = err.Error()
	return m, nil
}

func (m Model) onWorkloadsMsg(msg workloadsMsg) (tea.Model, tea.Cmd) {
	if m.staleFor(msg.poolIdx, msg.envIdx, msg.silent) {
		return m, nil
	}
	if msg.silent {
		m.autoRefreshBusy = false
		if m.rightView != viewWorkloads {
			return m, nil
		}
	}
	if m.rightView == viewWorkloads && m.namespace != "" && m.namespace != msg.namespace {
		return m, nil
	}
	m.loading = false
	m.rightView = viewWorkloads
	m.namespace = msg.namespace
	if !msg.silent {
		m.deployment = ""
		m.podName = ""
		m.podDetail = nil
		m.pods = nil
		m.rightCursor = 0
	}
	if msg.err != nil {
		m.err = msg.err.Error()
		m.workloads = nil
		return m, nil
	}
	m.err = ""
	m.workloads = msg.items
	m.rightCursor = clampIndex(m.rightCursor, len(m.workloads))
	return m, nil
}

func (m Model) onPodsMsg(msg podsMsg) (tea.Model, tea.Cmd) {
	if m.staleFor(msg.poolIdx, msg.envIdx, msg.silent) {
		return m, nil
	}
	if msg.silent {
		m.autoRefreshBusy = false
		if m.rightView != viewPods {
			return m, nil
		}
	}
	m.loading = false
	m.rightView = viewPods
	m.namespace = msg.namespace
	m.deployment = msg.deployment
	if !msg.silent {
		m.podName = ""
		m.podDetail = nil
		m.rightCursor = 0
	}
	if msg.err != nil {
		m.err = msg.err.Error()
		m.pods = nil
		return m, nil
	}
	m.err = ""
	m.pods = msg.items
	m.rightCursor = clampIndex(m.rightCursor, len(m.pods))
	return m, nil
}

func (m Model) onPodDetailMsg(msg podDetailMsg) (tea.Model, tea.Cmd) {
	if m.staleFor(msg.poolIdx, msg.envIdx, msg.silent) {
		return m, nil
	}
	if msg.silent {
		m.autoRefreshBusy = false
		if m.rightView != viewPodDetail {
			return m, nil
		}
	}
	m.loading = false
	m.rightView = viewPodDetail
	m.namespace = msg.namespace
	m.podName = msg.pod
	if !msg.silent {
		m.containerIdx = 0
		m.logText = ""
		m.logScroll = 0
		m.clearLogFilter()
	}
	if msg.err != nil {
		m.err = msg.err.Error()
		m.podDetail = nil
		return m, nil
	}
	m.err = ""
	m.podDetail = msg.detail
	if m.podDetail != nil {
		m.containerIdx = clampIndex(m.containerIdx, len(m.podDetail.Containers))
	}
	return m, nil
}

func (m Model) onLogChunk(msg logChunkMsg) (tea.Model, tea.Cmd) {
	if msg.id != m.logFollowID {
		if msg.ch != nil {
			return m, listenLogFollow(msg.ch)
		}
		return m, nil
	}
	if m.rightView != viewPodLogs {
		return m, listenLogFollow(msg.ch)
	}
	m.loading = false
	m.status = "following logs…"
	switch {
	case m.logText == "":
		m.logText = msg.text
	case strings.HasSuffix(m.logText, "\n"):
		m.logText += msg.text
	default:
		m.logText += "\n" + msg.text
	}
	if m.logStickBottom {
		m.scrollLogsToBottom()
	}
	return m, listenLogFollow(msg.ch)
}

func (m Model) onLogFollowDone(msg logFollowDoneMsg) (tea.Model, tea.Cmd) {
	if msg.id != m.logFollowID {
		return m, nil
	}
	if m.logFollowCmdID != 0 {
		m.finishCmd(m.logFollowCmdID, msg.err)
		m.logFollowCmdID = 0
	}
	m.loading = false
	if msg.err != nil && m.rightView == viewPodLogs {
		m.err = msg.err.Error()
		m.status = ""
	} else if m.rightView == viewPodLogs {
		m.status = ""
	}
	return m, m.maybeCmdAnim()
}

func (m Model) onEnsureMsg(msg ensureMsg) (tea.Model, tea.Cmd) {
	if msg.cmdID != 0 {
		m.finishCmd(msg.cmdID, msg.err)
	}
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
		return m, m.maybeCmdAnim()
	}
	m.loading = false
	kube.ClearClientCache()
	m.refreshAvailability()
	m.rebuildLeft()
	if msg.err != nil {
		m.err = msg.err.Error()
		m.status = ""
		return m, m.maybeCmdAnim()
	}
	m.err = ""
	m.status = "credentials updated"
	m.loading = true
	m.namespaces = nil
	m.workloads = nil
	m.pods = nil
	m.podDetail = nil
	m.rightCursor = 0
	if env.Namespace != "" {
		m.rightView = viewWorkloads
		m.namespace = env.Namespace
		line := m.kubectlLineWorkloads(env.Namespace)
		return m.tracked(line, false, func() tea.Msg {
			return m.fetchWorkloads(env, poolIdx, envIdx, env.Namespace)
		})
	}
	m.rightView = viewNamespaces
	m.namespace = ""
	return m.tracked(m.kubectlLineNamespaces(), false, func() tea.Msg {
		return m.fetchNamespaces(env, poolIdx, envIdx)
	})
}

func (m Model) onLoginMsg(msg loginMsg) (tea.Model, tea.Cmd) {
	if msg.cmdID != 0 {
		m.finishCmd(msg.cmdID, msg.err)
	}
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
		return m, m.maybeCmdAnim()
	}
	if msg.err != nil {
		applog.Infof("az login finished: error: %v", msg.err)
		m.loading = false
		m.err = msg.err.Error()
		m.status = ""
		return m, m.maybeCmdAnim()
	}
	applog.Infof("az login finished: ok")
	if env.Azure != nil && env.Azure.Subscription != "" {
		if err := azure.SetSubscription(env.Azure.Subscription); err != nil {
			m.loading = false
			m.err = err.Error()
			m.status = ""
			return m, nil
		}
	}
	m.err = ""
	m.status = "logged in · ensuring credentials…"
	return m.ensureCredentials()
}

func (m Model) onExecMsg(msg execMsg) (tea.Model, tea.Cmd) {
	if msg.cmdID != 0 {
		m.finishCmd(msg.cmdID, msg.err)
	}
	_, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
		return m, m.maybeCmdAnim()
	}
	m.loading = false
	m.status = ""
	if msg.err != nil {
		m.err = msg.err.Error()
	} else {
		m.err = ""
	}
	return m, m.maybeCmdAnim()
}

func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While filtering logs, capture keys before global hotkeys (r/c/a/…).
	// Only ctrl+c still quits so letters like q/r/c can be typed into the query.
	if m.rightView == viewPodLogs && m.logFilterActive {
		if msg.String() == "ctrl+c" {
			m.stopLogFollow()
			return m, tea.Quit
		}
		return m.updateLogFilter(msg)
	}

	if handled, model, cmd := m.handleGlobalKey(msg); handled {
		return model, cmd
	}

	if m.focus == paneLeft {
		return m.updateLeft(msg)
	}
	return m.updateRight(msg)
}

// handleGlobalKey processes hotkeys that apply regardless of pane focus.
// handled is false when the key should fall through to pane-specific handling.
func (m Model) handleGlobalKey(msg tea.KeyMsg) (handled bool, model tea.Model, cmd tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.stopLogFollow()
		return true, m, tea.Quit
	case "tab":
		if m.focus == paneLeft {
			m.focus = paneRight
		} else {
			m.focus = paneLeft
		}
		return true, m, nil
	case "r":
		model, cmd = m.refresh()
		return true, model, cmd
	case "l":
		if m.focus == paneLeft {
			model, cmd = m.ensureCredentials()
			return true, model, cmd
		}
	case "a":
		if m.focus == paneLeft {
			model, cmd = m.azLogin()
			return true, model, cmd
		}
	case "c":
		m.showCmds = !m.showCmds
		return true, m, m.maybeCmdAnim()
	case "g":
		if m.focus == paneRight && m.rightView == viewPodDetail {
			model, cmd = m.openPodLogs()
			return true, model, cmd
		}
	case "s":
		if m.focus == paneRight && m.rightView == viewPodDetail {
			model, cmd = m.execIntoContainer()
			return true, model, cmd
		}
	}
	return false, m, nil
}
