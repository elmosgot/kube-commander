package ui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/elmosgot/kube-commander/internal/applog"
	"github.com/elmosgot/kube-commander/internal/azure"
	"github.com/elmosgot/kube-commander/internal/config"
	"github.com/elmosgot/kube-commander/internal/kube"
)

const cmdsPanelHeight = 8

type pane int

const (
	paneLeft pane = iota
	paneRight
)

type rightView int

const (
	viewNamespaces rightView = iota
	viewWorkloads
	viewPods
	viewPodDetail
	viewPodLogs
)

type leftRow struct {
	poolIdx int
	envIdx  int // -1 for pool header rows
	label   string
	isPool  bool
}

type namespacesMsg struct {
	poolIdx int
	envIdx  int
	items   []kube.NamespaceInfo
	err     error
}

type workloadsMsg struct {
	poolIdx   int
	envIdx    int
	namespace string
	items     []kube.WorkloadInfo
	err       error
}

type podsMsg struct {
	poolIdx    int
	envIdx     int
	namespace  string
	deployment string
	items      []kube.PodInfo
	err        error
}

type podDetailMsg struct {
	poolIdx   int
	envIdx    int
	namespace string
	pod       string
	detail    *kube.PodDetail
	err       error
}

type podLogsMsg struct {
	poolIdx   int
	envIdx    int
	namespace string
	pod       string
	container string
	text      string
	err       error
}

type ensureMsg struct {
	poolIdx int
	envIdx  int
	err     error
}

type loginMsg struct {
	poolIdx int
	envIdx  int
	err     error
}

type execMsg struct {
	poolIdx int
	envIdx  int
	err     error
}

// Model is the Bubble Tea program state for the two-pane navigator.
type Model struct {
	cfg        *config.Config
	configPath string

	focus pane

	leftRows   []leftRow
	leftCursor int
	expanded   map[int]bool

	rightView   rightView
	rightCursor int
	namespaces  []kube.NamespaceInfo
	workloads   []kube.WorkloadInfo
	pods        []kube.PodInfo
	namespace   string
	deployment  string
	podName     string
	podDetail   *kube.PodDetail
	containerIdx int
	logText     string
	logScroll   int

	loading bool
	err     string
	status  string

	showCmds bool

	width  int
	height int

	spinner spinner.Model

	// availability of contexts; false means missing from kubeconfig
	available map[string]bool
}

// New creates a UI model from a loaded config.
func New(cfg *config.Config, configPath string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := Model{
		cfg:        cfg,
		configPath: configPath,
		focus:      paneLeft,
		expanded:   map[int]bool{},
		available:  map[string]bool{},
		spinner:    s,
	}
	for i := range cfg.Pools {
		m.expanded[i] = true
	}
	m.refreshAvailability()
	m.rebuildLeft()
	if len(m.leftRows) > 0 {
		for i, row := range m.leftRows {
			if !row.isPool {
				m.leftCursor = i
				break
			}
		}
	}
	return m
}

func envKey(env config.Environment) string {
	return config.ResolveKubeconfig(env) + "|" + env.Context
}

func (m *Model) refreshAvailability() {
	for i := range m.cfg.Pools {
		for _, env := range m.cfg.Pools[i].Environments {
			m.available[envKey(env)] = kube.ContextExists(env)
		}
	}
}

func (m *Model) rebuildLeft() {
	rows := make([]leftRow, 0)
	for i, pool := range m.cfg.Pools {
		rows = append(rows, leftRow{
			poolIdx: i,
			envIdx:  -1,
			label:   pool.Name,
			isPool:  true,
		})
		if !m.expanded[i] {
			continue
		}
		for j, env := range pool.Environments {
			mark := ""
			if !m.available[envKey(env)] {
				mark = " [unavailable]"
			}
			rows = append(rows, leftRow{
				poolIdx: i,
				envIdx:  j,
				label:   "  " + env.Name + mark,
				isPool:  false,
			})
		}
	}
	m.leftRows = rows
	if m.leftCursor >= len(m.leftRows) {
		m.leftCursor = len(m.leftRows) - 1
	}
	if m.leftCursor < 0 {
		m.leftCursor = 0
	}
}

func (m Model) selectedEnv() (config.Environment, int, int, bool) {
	if len(m.leftRows) == 0 {
		return config.Environment{}, 0, 0, false
	}
	row := m.leftRows[m.leftCursor]
	if row.isPool || row.envIdx < 0 {
		return config.Environment{}, row.poolIdx, -1, false
	}
	env := m.cfg.Pools[row.poolIdx].Environments[row.envIdx]
	return env, row.poolIdx, row.envIdx, true
}

func (m Model) selectedPoolName() string {
	if len(m.leftRows) == 0 {
		return ""
	}
	row := m.leftRows[m.leftCursor]
	return m.cfg.Pools[row.poolIdx].Name
}

func (m Model) selectedContainerName() string {
	if m.podDetail == nil || len(m.podDetail.Containers) == 0 {
		return ""
	}
	if m.containerIdx < 0 || m.containerIdx >= len(m.podDetail.Containers) {
		return m.podDetail.Containers[0].Name
	}
	return m.podDetail.Containers[m.containerIdx].Name
}

// Init starts the spinner and loads the initially selected environment.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.openSelectedEnv())
}

func (m Model) openSelectedEnv() tea.Cmd {
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok {
		return nil
	}
	if env.Namespace != "" {
		return func() tea.Msg {
			return m.fetchWorkloads(env, poolIdx, envIdx, env.Namespace)
		}
	}
	return func() tea.Msg {
		return m.fetchNamespaces(env, poolIdx, envIdx)
	}
}

func (m Model) loadNamespaces() tea.Cmd {
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		return m.fetchNamespaces(env, poolIdx, envIdx)
	}
}

func unavailableHint(env config.Environment) string {
	if env.Azure != nil && env.Azure.Subscription != "" {
		return fmt.Sprintf("context %q not found in kubeconfig (press a to az login, then l to ensure credentials)", env.Context)
	}
	if env.Azure != nil {
		return fmt.Sprintf("context %q not found in kubeconfig (press l to ensure credentials)", env.Context)
	}
	return fmt.Sprintf("context %q not found in kubeconfig", env.Context)
}

func (m Model) fetchNamespaces(env config.Environment, poolIdx, envIdx int) tea.Msg {
	if !kube.ContextExists(env) {
		return namespacesMsg{poolIdx: poolIdx, envIdx: envIdx, err: fmt.Errorf("%s", unavailableHint(env))}
	}
	client, err := kube.NewClient(env)
	if err != nil {
		return namespacesMsg{poolIdx: poolIdx, envIdx: envIdx, err: err}
	}
	items, err := client.ListNamespaces(context.Background())
	return namespacesMsg{poolIdx: poolIdx, envIdx: envIdx, items: items, err: err}
}

func (m Model) fetchWorkloads(env config.Environment, poolIdx, envIdx int, namespace string) tea.Msg {
	if !kube.ContextExists(env) {
		return workloadsMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, err: fmt.Errorf("%s", unavailableHint(env))}
	}
	client, err := kube.NewClient(env)
	if err != nil {
		return workloadsMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, err: err}
	}
	items, err := client.ListWorkloads(context.Background(), namespace)
	return workloadsMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, items: items, err: err}
}

func (m Model) fetchPods(env config.Environment, poolIdx, envIdx int, namespace, deployment string) tea.Msg {
	client, err := kube.NewClient(env)
	if err != nil {
		return podsMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, deployment: deployment, err: err}
	}
	items, err := client.ListPods(context.Background(), namespace, deployment)
	return podsMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, deployment: deployment, items: items, err: err}
}

func (m Model) fetchPodDetail(env config.Environment, poolIdx, envIdx int, namespace, pod string) tea.Msg {
	client, err := kube.NewClient(env)
	if err != nil {
		return podDetailMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, pod: pod, err: err}
	}
	detail, err := client.GetPod(context.Background(), namespace, pod)
	return podDetailMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, pod: pod, detail: detail, err: err}
}

func (m Model) fetchPodLogs(env config.Environment, poolIdx, envIdx int, namespace, pod, container string) tea.Msg {
	client, err := kube.NewClient(env)
	if err != nil {
		return podLogsMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, pod: pod, container: container, err: err}
	}
	text, err := client.GetPodLogs(context.Background(), namespace, pod, container, 200)
	return podLogsMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, pod: pod, container: container, text: text, err: err}
}

// Update handles keyboard and async messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case namespacesMsg:
		_, poolIdx, envIdx, ok := m.selectedEnv()
		if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
			return m, nil
		}
		m.loading = false
		m.rightView = viewNamespaces
		m.namespace = ""
		m.deployment = ""
		m.podName = ""
		m.podDetail = nil
		m.workloads = nil
		m.pods = nil
		m.rightCursor = 0
		if msg.err != nil {
			m.err = msg.err.Error()
			m.namespaces = nil
			return m, nil
		}
		m.err = ""
		m.namespaces = msg.items
		return m, nil

	case workloadsMsg:
		_, poolIdx, envIdx, ok := m.selectedEnv()
		if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
			return m, nil
		}
		if m.rightView == viewWorkloads && m.namespace != "" && m.namespace != msg.namespace {
			return m, nil
		}
		m.loading = false
		m.rightView = viewWorkloads
		m.namespace = msg.namespace
		m.deployment = ""
		m.podName = ""
		m.podDetail = nil
		m.pods = nil
		m.rightCursor = 0
		if msg.err != nil {
			m.err = msg.err.Error()
			m.workloads = nil
			return m, nil
		}
		m.err = ""
		m.workloads = msg.items
		return m, nil

	case podsMsg:
		_, poolIdx, envIdx, ok := m.selectedEnv()
		if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
			return m, nil
		}
		m.loading = false
		m.rightView = viewPods
		m.namespace = msg.namespace
		m.deployment = msg.deployment
		m.podName = ""
		m.podDetail = nil
		m.rightCursor = 0
		if msg.err != nil {
			m.err = msg.err.Error()
			m.pods = nil
			return m, nil
		}
		m.err = ""
		m.pods = msg.items
		return m, nil

	case podDetailMsg:
		_, poolIdx, envIdx, ok := m.selectedEnv()
		if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
			return m, nil
		}
		m.loading = false
		m.rightView = viewPodDetail
		m.namespace = msg.namespace
		m.podName = msg.pod
		m.containerIdx = 0
		m.logText = ""
		m.logScroll = 0
		if msg.err != nil {
			m.err = msg.err.Error()
			m.podDetail = nil
			return m, nil
		}
		m.err = ""
		m.podDetail = msg.detail
		return m, nil

	case podLogsMsg:
		_, poolIdx, envIdx, ok := m.selectedEnv()
		if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.rightView = viewPodLogs
		m.logText = msg.text
		m.logScroll = 0
		return m, nil

	case ensureMsg:
		env, poolIdx, envIdx, ok := m.selectedEnv()
		if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
			return m, nil
		}
		m.loading = false
		kube.ClearClientCache()
		m.refreshAvailability()
		m.rebuildLeft()
		if msg.err != nil {
			m.err = msg.err.Error()
			m.status = ""
			return m, nil
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
			return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
				return m.fetchWorkloads(env, poolIdx, envIdx, env.Namespace)
			})
		}
		m.rightView = viewNamespaces
		m.namespace = ""
		return m, tea.Batch(m.spinner.Tick, m.loadNamespaces())

	case loginMsg:
		env, poolIdx, envIdx, ok := m.selectedEnv()
		if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
			return m, nil
		}
		if msg.err != nil {
			applog.Infof("az login finished: error: %v", msg.err)
			m.loading = false
			m.err = msg.err.Error()
			m.status = ""
			return m, nil
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

	case execMsg:
		_, poolIdx, envIdx, ok := m.selectedEnv()
		if !ok || poolIdx != msg.poolIdx || envIdx != msg.envIdx {
			return m, nil
		}
		m.loading = false
		m.status = ""
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.err = ""
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.focus == paneLeft {
				m.focus = paneRight
			} else {
				m.focus = paneLeft
			}
			return m, nil
		case "r":
			return m.refresh()
		case "l":
			return m.ensureCredentials()
		case "a":
			return m.azLogin()
		case "c":
			m.showCmds = !m.showCmds
			return m, nil
		case "g":
			if m.focus == paneRight && m.rightView == viewPodDetail {
				return m.openPodLogs()
			}
		case "s":
			if m.focus == paneRight && m.rightView == viewPodDetail {
				return m.execIntoContainer()
			}
		}

		if m.focus == paneLeft {
			return m.updateLeft(msg)
		}
		return m.updateRight(msg)
	}
	return m, nil
}

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
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		err := azure.EnsureCredentials(env)
		return ensureMsg{poolIdx: poolIdx, envIdx: envIdx, err: err}
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
	if supportsLoginSub {
		m.status = "az login -s " + env.Azure.Subscription + "…"
	} else {
		m.status = "az login…"
	}
	cmd := azure.PrepareLoginCmd(env.Azure.Subscription)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return loginMsg{poolIdx: poolIdx, envIdx: envIdx, err: err}
	})
}

func (m Model) openPodLogs() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok || m.podDetail == nil {
		return m, nil
	}
	container := m.selectedContainerName()
	if container == "" {
		m.err = "no container selected"
		return m, nil
	}
	m.loading = true
	m.err = ""
	m.status = "fetching logs…"
	ns := m.namespace
	pod := m.podName
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		return m.fetchPodLogs(env, poolIdx, envIdx, ns, pod, container)
	})
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
	applog.Infof("starting interactive: kubectl %s", strings.Join(args, " "))
	m.loading = true
	m.err = ""
	m.status = fmt.Sprintf("exec %s / %s…", m.podName, container)
	cmd := exec.Command("kubectl", args...)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execMsg{poolIdx: poolIdx, envIdx: envIdx, err: err}
	})
}

// kubectlExecArgs builds kubectl exec -it … -- sh -c 'bash || sh'.
func kubectlExecArgs(env config.Environment, namespace, pod, container string) []string {
	args := []string{
		"exec", "-it",
		"--context", env.Context,
		"--kubeconfig", config.ResolveKubeconfig(env),
		"-n", namespace,
		pod,
		"-c", container,
		"--",
		"sh", "-c", "command -v bash >/dev/null 2>&1 && exec bash || exec sh",
	}
	return args
}

func (m Model) refresh() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok {
		return m, nil
	}
	m.loading = true
	m.err = ""
	m.status = ""
	switch m.rightView {
	case viewPodLogs:
		ns, pod, ctr := m.namespace, m.podName, m.selectedContainerName()
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			return m.fetchPodLogs(env, poolIdx, envIdx, ns, pod, ctr)
		})
	case viewPodDetail:
		ns, pod := m.namespace, m.podName
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			return m.fetchPodDetail(env, poolIdx, envIdx, ns, pod)
		})
	case viewPods:
		ns, dep := m.namespace, m.deployment
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			return m.fetchPods(env, poolIdx, envIdx, ns, dep)
		})
	case viewWorkloads:
		ns := m.namespace
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			return m.fetchWorkloads(env, poolIdx, envIdx, ns)
		})
	default:
		return m, tea.Batch(m.spinner.Tick, m.loadNamespaces())
	}
}

func (m Model) updateLeft(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.leftCursor > 0 {
			m.leftCursor--
		}
		return m, nil
	case "down", "j":
		if m.leftCursor < len(m.leftRows)-1 {
			m.leftCursor++
		}
		return m, nil
	case "enter", "right":
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
		m.loading = true
		m.err = ""
		m.status = ""
		m.namespaces = nil
		m.workloads = nil
		m.pods = nil
		m.podDetail = nil
		m.deployment = ""
		m.podName = ""
		m.rightCursor = 0
		m.focus = paneRight
		if env.Namespace != "" {
			m.rightView = viewWorkloads
			m.namespace = env.Namespace
			return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
				return m.fetchWorkloads(env, row.poolIdx, row.envIdx, env.Namespace)
			})
		}
		m.rightView = viewNamespaces
		m.namespace = ""
		return m, tea.Batch(m.spinner.Tick, m.loadNamespaces())
	case "left", "h":
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
	return m, nil
}

func (m Model) updateRight(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		return m.moveRight(-1)
	case "down", "j":
		return m.moveRight(1)
	case "pgup":
		return m.moveRight(-10)
	case "pgdown":
		return m.moveRight(10)
	case "enter", "right":
		return m.openRight()
	case "backspace", "left", "h", "esc":
		return m.backRight()
	}
	return m, nil
}

func (m Model) moveRight(delta int) (tea.Model, tea.Cmd) {
	if m.rightView == viewPodLogs {
		lines := strings.Split(m.logText, "\n")
		maxScroll := len(lines) - 1
		if maxScroll < 0 {
			maxScroll = 0
		}
		m.logScroll += delta
		if m.logScroll < 0 {
			m.logScroll = 0
		}
		if m.logScroll > maxScroll {
			m.logScroll = maxScroll
		}
		return m, nil
	}
	if m.rightView == viewPodDetail {
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
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			return m.fetchWorkloads(env, poolIdx, envIdx, ns)
		})
	case viewWorkloads:
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
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			return m.fetchPods(env, poolIdx, envIdx, ns, dep)
		})
	case viewPods:
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
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			return m.fetchPodDetail(env, poolIdx, envIdx, ns, pod)
		})
	case viewPodDetail:
		return m.openPodLogs()
	}
	return m, nil
}

func (m Model) backRight() (tea.Model, tea.Cmd) {
	switch m.rightView {
	case viewPodLogs:
		m.rightView = viewPodDetail
		m.logText = ""
		m.logScroll = 0
		m.err = ""
		m.status = ""
		return m, nil
	case viewPodDetail:
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
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			return m.fetchPods(env, poolIdx, envIdx, ns, dep)
		})
	case viewPods:
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
		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			return m.fetchWorkloads(env, poolIdx, envIdx, ns)
		})
	case viewWorkloads:
		m.rightView = viewNamespaces
		m.namespace = ""
		m.workloads = nil
		m.rightCursor = 0
		m.err = ""
		m.status = ""
		m.loading = true
		return m, tea.Batch(m.spinner.Tick, m.loadNamespaces())
	default:
		m.focus = paneLeft
		return m, nil
	}
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
		return len(strings.Split(m.logText, "\n"))
	default:
		return len(m.namespaces)
	}
}

// layoutHeights returns body and commands panel heights that fit in terminalH.
// Chrome is header (1) + footer (1) + help (1). Never inflates past the terminal.
func layoutHeights(terminalH int, showCmds bool) (bodyH, cmdsH int) {
	const chrome = 3 // header + footer + help
	avail := terminalH - chrome
	if avail < 1 {
		avail = 1
	}
	if !showCmds {
		return avail, 0
	}
	cmdsH = cmdsPanelHeight
	if cmdsH > avail-1 {
		cmdsH = avail - 1
	}
	if cmdsH < 1 {
		cmdsH = 1
	}
	bodyH = avail - cmdsH
	if bodyH < 1 {
		bodyH = 1
		cmdsH = avail - bodyH
		if cmdsH < 0 {
			cmdsH = 0
		}
	}
	return bodyH, cmdsH
}

// View renders the two panes and footer.
func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	bodyH, cmdsH := layoutHeights(m.height, m.showCmds)

	leftW := m.width / 3
	if leftW < 24 {
		leftW = 24
	}
	if leftW > 40 {
		leftW = 40
	}
	rightW := m.width - leftW - 1
	if rightW < 20 {
		rightW = 20
	}

	left := m.renderLeft(leftW, bodyH)
	right := m.renderRight(rightW, bodyH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	header := titleStyle.Render("kube-commander") + "  " + mutedStyle.Render(truncate(m.configPath, m.width-20))
	footer := m.renderFooter()
	help := helpStyle.Render(truncate("tab focus · ↑↓ move · enter open · g logs · s shell · a az login · l ensure · c commands · backspace back · r refresh · q quit", m.width))

	parts := []string{header, body}
	if m.showCmds && cmdsH > 0 {
		parts = append(parts, m.renderCmdsPanel(m.width, cmdsH))
	}
	parts = append(parts, footer, help)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderCmdsPanel(width, height int) string {
	inner := height - 2 // border
	if inner < 1 {
		inner = 1
	}
	var b strings.Builder
	linesLeft := inner
	writeLine := func(s string) {
		if linesLeft <= 0 {
			return
		}
		b.WriteString(s + "\n")
		linesLeft--
	}
	writeLine(sectionStyle.Render("View"))
	for _, line := range m.kubectlRecipes() {
		writeLine(mutedStyle.Render(truncate(line, width-4)))
	}
	writeLine(sectionStyle.Render("Shell"))
	shell := applog.RecentCmds()
	if len(shell) == 0 {
		writeLine(mutedStyle.Render("(no shell commands yet)"))
	} else {
		if len(shell) > linesLeft {
			shell = shell[len(shell)-linesLeft:]
		}
		for _, line := range shell {
			writeLine(mutedStyle.Render(truncate("$ "+line, width-4)))
		}
	}
	return fitBox(paneStyle, width, height, b.String())
}

func fitBox(style lipgloss.Style, width, height int, content string) string {
	return style.Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(content)
}

func (m Model) renderLeft(width, height int) string {
	border := paneStyle
	if m.focus == paneLeft {
		border = paneFocusedStyle
	}
	var b strings.Builder
	b.WriteString(sectionStyle.Render("Pools") + "\n")
	innerRows := height - 2 // border top/bottom
	if innerRows < 1 {
		innerRows = 1
	}
	written := 1 // title
	for i, row := range m.leftRows {
		if written >= innerRows {
			break
		}
		line := row.label
		if row.isPool {
			marker := "▾"
			if !m.expanded[row.poolIdx] {
				marker = "▸"
			}
			line = marker + " " + row.label
		}
		style := itemStyle
		if i == m.leftCursor {
			if m.focus == paneLeft {
				style = selectedStyle
			} else {
				style = selectedDimStyle
			}
		} else if !row.isPool {
			env := m.cfg.Pools[row.poolIdx].Environments[row.envIdx]
			if !m.available[envKey(env)] {
				style = unavailableStyle
			}
		}
		b.WriteString(style.Width(width - 4).Render(truncate(line, width-4)))
		b.WriteByte('\n')
		written++
	}
	return fitBox(border, width, height, b.String())
}

func (m Model) renderRight(width, height int) string {
	border := paneStyle
	if m.focus == paneRight {
		border = paneFocusedStyle
	}

	var title string
	switch m.rightView {
	case viewWorkloads:
		title = fmt.Sprintf("Workloads · %s", m.namespace)
	case viewPods:
		title = fmt.Sprintf("Pods · %s", m.deployment)
	case viewPodDetail:
		title = fmt.Sprintf("Pod · %s", m.podName)
	case viewPodLogs:
		title = fmt.Sprintf("Logs · %s / %s", m.podName, m.selectedContainerName())
	default:
		title = "Namespaces"
	}

	var b strings.Builder
	b.WriteString(sectionStyle.Render(title) + "\n")

	if m.loading {
		b.WriteString(m.spinner.View() + " loading…\n")
	} else if m.err != "" && m.rightView != viewPodLogs && m.rightLen() == 0 {
		b.WriteString(errorStyle.Render(truncate(m.err, width-4)) + "\n")
	} else {
		switch m.rightView {
		case viewNamespaces:
			m.renderNamespaceRows(&b, width)
		case viewWorkloads:
			m.renderWorkloadRows(&b, width)
		case viewPods:
			m.renderPodRows(&b, width)
		case viewPodDetail:
			m.renderPodDetail(&b, width)
		case viewPodLogs:
			m.renderPodLogs(&b, width, height)
		}
	}

	return fitBox(border, width, height, b.String())
}

func (m Model) rowStyle(selected bool) lipgloss.Style {
	if !selected {
		return itemStyle
	}
	if m.focus == paneRight {
		return selectedStyle
	}
	return selectedDimStyle
}

func (m Model) renderNamespaceRows(b *strings.Builder, width int) {
	if len(m.namespaces) == 0 {
		b.WriteString(mutedStyle.Render("no namespaces") + "\n")
		return
	}
	for i, ns := range m.namespaces {
		line := fmt.Sprintf("%-24s  deps %-3d  pods %d/%d",
			ns.Name, ns.DeploymentCount, ns.ReadyPods, ns.TotalPods)
		b.WriteString(m.rowStyle(i == m.rightCursor).Width(width - 4).Render(truncate(line, width-4)))
		b.WriteByte('\n')
	}
}

func (m Model) renderWorkloadRows(b *strings.Builder, width int) {
	if len(m.workloads) == 0 {
		b.WriteString(mutedStyle.Render("no deployments") + "\n")
		return
	}
	for i, w := range m.workloads {
		line := fmt.Sprintf("%-28s  %d/%d  age %s", w.Name, w.Ready, w.Desired, w.Age)
		b.WriteString(m.rowStyle(i == m.rightCursor).Width(width - 4).Render(truncate(line, width-4)))
		b.WriteByte('\n')
	}
}

func (m Model) renderPodRows(b *strings.Builder, width int) {
	if len(m.pods) == 0 {
		b.WriteString(mutedStyle.Render("no pods") + "\n")
		return
	}
	for i, p := range m.pods {
		line := fmt.Sprintf("%-36s  %-5s  restarts %-3d  %-12s  %s",
			p.Name, p.Ready, p.Restarts, p.Status, p.Age)
		b.WriteString(m.rowStyle(i == m.rightCursor).Width(width - 4).Render(truncate(line, width-4)))
		b.WriteByte('\n')
	}
}

func (m Model) renderPodDetail(b *strings.Builder, width int) {
	if m.podDetail == nil {
		b.WriteString(mutedStyle.Render("no pod detail") + "\n")
		return
	}
	d := m.podDetail
	b.WriteString(fmt.Sprintf("phase=%s  ready=%s  node=%s  ip=%s  age=%s\n",
		d.Phase, d.Ready, d.Node, d.PodIP, d.Age))
	b.WriteString(mutedStyle.Render("containers (enter/g = logs)") + "\n")
	for i, c := range d.Containers {
		ready := "no"
		if c.Ready {
			ready = "yes"
		}
		line := fmt.Sprintf("%-20s  %-7s  rst %-3d  %-12s  cpu %s/%s  mem %s/%s",
			c.Name, ready, c.Restarts, c.State, c.CPURequest, c.CPULimit, c.MemRequest, c.MemLimit)
		b.WriteString(m.rowStyle(i == m.containerIdx).Width(width - 4).Render(truncate(line, width-4)))
		b.WriteByte('\n')
		img := "  image " + c.Image
		b.WriteString(mutedStyle.Width(width - 4).Render(truncate(img, width-4)))
		b.WriteByte('\n')
	}
}

func (m Model) renderPodLogs(b *strings.Builder, width, height int) {
	if m.err != "" && m.logText == "" {
		b.WriteString(errorStyle.Render(truncate(m.err, width-4)) + "\n")
		return
	}
	lines := strings.Split(m.logText, "\n")
	// Reserve title line already written; keep a few rows of padding inside the border.
	visible := height - 4
	if visible < 3 {
		visible = 3
	}
	start := m.logScroll
	if start < 0 {
		start = 0
	}
	if start > len(lines) {
		start = len(lines)
	}
	end := start + visible
	if end > len(lines) {
		end = len(lines)
	}
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		b.WriteString(mutedStyle.Render("no log output") + "\n")
		return
	}
	for _, line := range lines[start:end] {
		b.WriteString(itemStyle.Width(width - 4).Render(truncate(line, width-4)))
		b.WriteByte('\n')
	}
}

func (m Model) renderFooter() string {
	pool := m.selectedPoolName()
	env, _, _, ok := m.selectedEnv()
	parts := []string{}
	if pool != "" {
		parts = append(parts, "pool="+pool)
	}
	if ok {
		parts = append(parts, "env="+env.Name, "context="+env.Context)
		ns := m.namespace
		if ns == "" {
			ns = env.Namespace
		}
		if ns != "" {
			parts = append(parts, "namespace="+ns)
		}
		if m.deployment != "" {
			parts = append(parts, "deploy="+m.deployment)
		}
		if m.podName != "" {
			parts = append(parts, "pod="+m.podName)
		}
		if ctr := m.selectedContainerName(); ctr != "" && (m.rightView == viewPodDetail || m.rightView == viewPodLogs) {
			parts = append(parts, "container="+ctr)
		}
		parts = append(parts, "kubeconfig="+config.ResolveKubeconfig(env))
	} else {
		parts = append(parts, "select an environment")
	}
	line := strings.Join(parts, "  ·  ")
	if m.status != "" {
		line += "  ·  " + m.status
	}
	if m.err != "" {
		line += "  ·  " + m.err
	}
	// Single visual row so the height budget stays valid.
	w := m.width
	if w <= 0 {
		w = 80
	}
	line = truncate(line, w)
	if m.err != "" && strings.Contains(line, m.err) {
		idx := strings.LastIndex(line, m.err)
		return footerStyle.Render(line[:idx]) + errorStyle.Render(line[idx:])
	}
	return footerStyle.Render(line)
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return string(runes[:n])
	}
	return string(runes[:n-1]) + "…"
}

var (
	titleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	mutedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	footerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	sectionStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	itemStyle        = lipgloss.NewStyle()
	selectedStyle    = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
	selectedDimStyle = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
	unavailableStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Italic(true)
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	paneStyle        = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	paneFocusedStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1)
)
