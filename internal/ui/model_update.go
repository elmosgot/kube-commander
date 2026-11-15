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
func (model Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return model.onWindowSize(msg)
	case tea.MouseMsg:
		return model.onMouse(msg)
	case spinner.TickMsg:
		return model.onSpinnerTick(msg)
	case cmdAnimMsg:
		return model.onCmdAnim()
	case cmdDoneMsg:
		return model.onCmdDone(msg)
	case initFetchMsg:
		return model.openSelectedEnvTracked()
	case autoRefreshMsg:
		return model.onAutoRefresh()
	case namespacesMsg:
		return model.onNamespacesMsg(msg)
	case workloadsMsg:
		return model.onWorkloadsMsg(msg)
	case podsMsg:
		return model.onPodsMsg(msg)
	case podDetailMsg:
		return model.onPodDetailMsg(msg)
	case logChunkMsg:
		return model.onLogChunk(msg)
	case logFollowDoneMsg:
		return model.onLogFollowDone(msg)
	case ensureMsg:
		return model.onEnsureMsg(msg)
	case loginMsg:
		return model.onLoginMsg(msg)
	case execMsg:
		return model.onExecMsg(msg)
	case tea.KeyMsg:
		return model.onKey(msg)
	}
	return model, nil
}

func (model Model) onWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	model.width = msg.Width
	model.height = msg.Height
	return model, nil
}

func (model Model) onMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft || !msg.Ctrl {
		return model, nil
	}
	if model.showCmds {
		if handled, mm := model.ctrlClickCmdLog(msg.X, msg.Y); handled {
			return mm, nil
		}
	}
	if model.rightView == viewPodLogs {
		if idx, ok := model.logLogicalIdxAtScreen(msg.X, msg.Y); ok {
			model.copyLogLogicalLine(idx)
		}
	}
	return model, nil
}

// ctrlClickCmdLog copies the clicked command-log line; handled is false when the
// click did not land on a command entry.
func (model Model) ctrlClickCmdLog(x, y int) (handled bool, out Model) {
	id, ok := model.cmdLogIDAtScreen(x, y)
	if !ok {
		return false, model
	}
	e, found := cmdEntryByID(model.cmdLog, id)
	if !found {
		return false, model
	}
	if err := copyToClipboard(e.Line); err != nil {
		model.status = "clipboard failed: " + err.Error()
	} else {
		model.status = "copied"
		model.err = ""
	}
	return true, model
}

func (model Model) onSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	model.spinner, cmd = model.spinner.Update(msg)
	return model, tea.Batch(cmd, model.maybeCmdAnim())
}

func (model Model) onCmdAnim() (tea.Model, tea.Cmd) {
	model.cmdAnimFrame++
	return model, model.maybeCmdAnim()
}

func (model Model) onCmdDone(msg cmdDoneMsg) (tea.Model, tea.Cmd) {
	model.finishCmd(msg.id, msg.err)
	inner, cmd := model.Update(msg.inner)
	m2 := inner.(Model)
	return m2, tea.Batch(cmd, m2.maybeCmdAnim())
}

func (model Model) onAutoRefresh() (tea.Model, tea.Cmd) {
	next := scheduleAutoRefresh()
	if model.rightView == viewPodLogs || model.loading || model.autoRefreshBusy {
		return model, next
	}
	var cmd tea.Cmd
	model, cmd = model.quietRefresh()
	if cmd == nil {
		return model, next
	}
	model.autoRefreshBusy = true
	return model, tea.Batch(next, cmd)
}

// staleFor reports whether an async message targets a different environment than
// the current selection, and clears the busy flag for silent (auto-refresh) msgs.
func (model *Model) staleFor(poolIdx, envIdx int, silent bool) bool {
	_, curPool, curEnv, ok := model.selectedEnv()
	if ok && curPool == poolIdx && curEnv == envIdx {
		return false
	}
	if silent {
		model.autoRefreshBusy = false
	}
	return true
}

func (model Model) onNamespacesMsg(msg namespacesMsg) (tea.Model, tea.Cmd) {
	if model.staleFor(msg.poolIdx, msg.envIdx, msg.silent) {
		return model, nil
	}
	if msg.silent {
		model.autoRefreshBusy = false
		if model.rightView != viewNamespaces {
			return model, nil
		}
	}
	model.loading = false
	if !msg.silent {
		model.deployment = ""
		model.podName = ""
		model.podDetail = nil
		model.workloads = nil
		model.pods = nil
		model.rightCursor = 0
	}
	if msg.err != nil {
		return model.namespacesError(msg.err)
	}
	model.rightView = viewNamespaces
	model.namespace = ""
	model.err = ""
	model.namespaces = msg.items
	model.rightCursor = clampIndex(model.rightCursor, len(model.namespaces))
	return model, nil
}

func (model Model) namespacesError(err error) (tea.Model, tea.Cmd) {
	model.namespaces = nil
	if kube.IsForbidden(err) {
		model.err = "cannot list namespaces (RBAC). Set environments[].namespace in config, then reopen the environment."
		model.focus = paneLeft
		model.namespace = ""
		model.status = ""
		return model, nil
	}
	model.rightView = viewNamespaces
	model.namespace = ""
	model.err = err.Error()
	return model, nil
}

func (model Model) onWorkloadsMsg(msg workloadsMsg) (tea.Model, tea.Cmd) {
	if model.staleFor(msg.poolIdx, msg.envIdx, msg.silent) {
		return model, nil
	}
	if msg.silent {
		model.autoRefreshBusy = false
		if model.rightView != viewWorkloads {
			return model, nil
		}
	}
	if model.rightView == viewWorkloads && model.namespace != "" && model.namespace != msg.namespace {
		return model, nil
	}
	model.loading = false
	model.rightView = viewWorkloads
	model.namespace = msg.namespace
	if !msg.silent {
		model.deployment = ""
		model.podName = ""
		model.podDetail = nil
		model.pods = nil
		model.rightCursor = 0
	}
	if msg.err != nil {
		model.err = msg.err.Error()
		model.workloads = nil
		return model, nil
	}
	model.err = ""
	model.workloads = msg.items
	model.rightCursor = clampIndex(model.rightCursor, len(model.workloads))
	return model, nil
}

func (model Model) onPodsMsg(msg podsMsg) (tea.Model, tea.Cmd) {
	if model.staleFor(msg.poolIdx, msg.envIdx, msg.silent) {
		return model, nil
	}
	if msg.silent {
		model.autoRefreshBusy = false
		if model.rightView != viewPods {
			return model, nil
		}
	}
	model.loading = false
	model.rightView = viewPods
	model.namespace = msg.namespace
	model.deployment = msg.deployment
	if !msg.silent {
		model.podName = ""
		model.podDetail = nil
		model.rightCursor = 0
	}
	if msg.err != nil {
		model.err = msg.err.Error()
		model.pods = nil
		return model, nil
	}
	model.err = ""
	model.pods = msg.items
	model.rightCursor = clampIndex(model.rightCursor, len(model.pods))
	return model, nil
}

func (model Model) onPodDetailMsg(msg podDetailMsg) (tea.Model, tea.Cmd) {
	if model.staleFor(msg.poolIdx, msg.envIdx, msg.silent) {
		return model, nil
	}
	if msg.silent {
		model.autoRefreshBusy = false
		if model.rightView != viewPodDetail {
			return model, nil
		}
	}
	model.loading = false
	model.rightView = viewPodDetail
	model.namespace = msg.namespace
	model.podName = msg.pod
	if !msg.silent {
		model.containerIdx = 0
		model.logText = ""
		model.logScroll = 0
		model.clearLogFilter()
	}
	if msg.err != nil {
		model.err = msg.err.Error()
		model.podDetail = nil
		return model, nil
	}
	model.err = ""
	model.podDetail = msg.detail
	if model.podDetail != nil {
		model.containerIdx = clampIndex(model.containerIdx, len(model.podDetail.Containers))
	}
	return model, nil
}

func (model Model) onLogChunk(msg logChunkMsg) (tea.Model, tea.Cmd) {
	if msg.id != model.logFollowID {
		if msg.ch != nil {
			return model, listenLogFollow(msg.ch)
		}
		return model, nil
	}
	if model.rightView != viewPodLogs {
		return model, listenLogFollow(msg.ch)
	}
	model.loading = false
	model.status = "following logs…"
	switch {
	case model.logText == "":
		model.logText = msg.text
	case strings.HasSuffix(model.logText, "\n"):
		model.logText += msg.text
	default:
		model.logText += "\n" + msg.text
	}
	if model.logStickBottom {
		model.scrollLogsToBottom()
	}
	return model, listenLogFollow(msg.ch)
}

func (model Model) onLogFollowDone(msg logFollowDoneMsg) (tea.Model, tea.Cmd) {
	if msg.id != model.logFollowID {
		return model, nil
	}
	if model.logFollowCmdID != 0 {
		model.finishCmd(model.logFollowCmdID, msg.err)
		model.logFollowCmdID = 0
	}
	model.loading = false
	if msg.err != nil && model.rightView == viewPodLogs {
		model.err = msg.err.Error()
		model.status = ""
	} else if model.rightView == viewPodLogs {
		model.status = ""
	}
	return model, model.maybeCmdAnim()
}

func (model Model) onEnsureMsg(msg ensureMsg) (tea.Model, tea.Cmd) {
	if msg.cmdID != 0 {
		model.finishCmd(msg.cmdID, msg.err)
	}
	env, poolIdx, envIdx, ok := model.selectedEnv()
	if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
		return model, model.maybeCmdAnim()
	}
	model.loading = false
	kube.ClearClientCache()
	model.refreshAvailability()
	model.rebuildLeft()
	if msg.err != nil {
		model.err = msg.err.Error()
		model.status = ""
		return model, model.maybeCmdAnim()
	}
	model.err = ""
	model.status = "credentials updated"
	model.loading = true
	model.namespaces = nil
	model.workloads = nil
	model.pods = nil
	model.podDetail = nil
	model.rightCursor = 0
	if env.Namespace != "" {
		model.rightView = viewWorkloads
		model.namespace = env.Namespace
		line := model.kubectlLineWorkloads(env.Namespace)
		return model.tracked(line, false, func() tea.Msg {
			return model.fetchWorkloads(env, poolIdx, envIdx, env.Namespace)
		})
	}
	model.rightView = viewNamespaces
	model.namespace = ""
	return model.tracked(model.kubectlLineNamespaces(), false, func() tea.Msg {
		return model.fetchNamespaces(env, poolIdx, envIdx)
	})
}

func (model Model) onLoginMsg(msg loginMsg) (tea.Model, tea.Cmd) {
	if msg.cmdID != 0 {
		model.finishCmd(msg.cmdID, msg.err)
	}
	env, poolIdx, envIdx, ok := model.selectedEnv()
	if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
		return model, model.maybeCmdAnim()
	}
	if msg.err != nil {
		applog.Infof("az login finished: error: %v", msg.err)
		model.loading = false
		model.err = msg.err.Error()
		model.status = ""
		return model, model.maybeCmdAnim()
	}
	applog.Infof("az login finished: ok")
	if env.Azure != nil && env.Azure.Subscription != "" {
		if err := azure.SetSubscription(env.Azure.Subscription); err != nil {
			model.loading = false
			model.err = err.Error()
			model.status = ""
			return model, nil
		}
	}
	model.err = ""
	model.status = "logged in · ensuring credentials…"
	return model.ensureCredentials()
}

func (model Model) onExecMsg(msg execMsg) (tea.Model, tea.Cmd) {
	if msg.cmdID != 0 {
		model.finishCmd(msg.cmdID, msg.err)
	}
	_, poolIdx, envIdx, ok := model.selectedEnv()
	if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
		return model, model.maybeCmdAnim()
	}
	model.loading = false
	model.status = ""
	if msg.err != nil {
		model.err = msg.err.Error()
	} else {
		model.err = ""
	}
	return model, model.maybeCmdAnim()
}

func (model Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While filtering logs, capture keys before global hotkeys (r/c/a/…).
	// Only ctrl+c still quits so letters like q/r/c can be typed into the query.
	if model.rightView == viewPodLogs && model.logFilterActive {
		if msg.String() == "ctrl+c" {
			model.stopLogFollow()
			return model, tea.Quit
		}
		return model.updateLogFilter(msg)
	}

	if handled, model, cmd := model.handleGlobalKey(msg); handled {
		return model, cmd
	}

	if model.focus == paneLeft {
		return model.updateLeft(msg)
	}
	return model.updateRight(msg)
}

// handleGlobalKey processes hotkeys that apply regardless of pane focus.
// handled is false when the key should fall through to pane-specific handling.
func (model Model) handleGlobalKey(msg tea.KeyMsg) (handled bool, out tea.Model, cmd tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		model.stopLogFollow()
		return true, model, tea.Quit
	case "tab":
		if model.focus == paneLeft {
			model.focus = paneRight
		} else {
			model.focus = paneLeft
		}
		return true, model, nil
	case "r":
		out, cmd = model.refresh()
		return true, out, cmd
	case "l":
		if model.focus == paneLeft {
			out, cmd = model.ensureCredentials()
			return true, out, cmd
		}
	case "a":
		if model.focus == paneLeft {
			out, cmd = model.azLogin()
			return true, out, cmd
		}
	case "c":
		model.showCmds = !model.showCmds
		return true, model, model.maybeCmdAnim()
	case "g":
		if model.focus == paneRight && model.rightView == viewPodDetail {
			out, cmd = model.openPodLogs()
			return true, out, cmd
		}
	case "s":
		if model.focus == paneRight && model.rightView == viewPodDetail {
			out, cmd = model.execIntoContainer()
			return true, out, cmd
		}
	}
	return false, model, nil
}
