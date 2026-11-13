package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/elmosgot/kube-commander/internal/config"
	"github.com/elmosgot/kube-commander/internal/kube"
)

const (
	cmdsPanelHeight     = 8
	autoRefreshInterval = 5 * time.Second
)

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
	silent  bool
}

type workloadsMsg struct {
	poolIdx   int
	envIdx    int
	namespace string
	items     []kube.WorkloadInfo
	err       error
	silent    bool
}

type podsMsg struct {
	poolIdx    int
	envIdx     int
	namespace  string
	deployment string
	items      []kube.PodInfo
	err        error
	silent     bool
}

type podDetailMsg struct {
	poolIdx   int
	envIdx    int
	namespace string
	pod       string
	detail    *kube.PodDetail
	err       error
	silent    bool
}

type autoRefreshMsg struct{}

type ensureMsg struct {
	poolIdx int
	envIdx  int
	cmdID   int
	err     error
}

type loginMsg struct {
	poolIdx int
	envIdx  int
	cmdID   int
	err     error
}

type execMsg struct {
	poolIdx int
	envIdx  int
	cmdID   int
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

	rightView       rightView
	rightCursor     int
	namespaces      []kube.NamespaceInfo
	workloads       []kube.WorkloadInfo
	pods            []kube.PodInfo
	namespace       string
	deployment      string
	podName         string
	podDetail       *kube.PodDetail
	containerIdx    int
	logText         string
	logScroll       int
	logFilterActive bool
	logFilter       string
	logStickBottom  bool
	logFollowID     int
	logFollowCancel context.CancelFunc
	logFollowCmdID  int // cmd log entry for active follow stream

	loading         bool
	autoRefreshBusy bool
	err             string
	status          string

	showCmds     bool
	cmdLog       []cmdEntry
	cmdLogSeq    int
	cmdAnimFrame int

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

type initFetchMsg struct{}

// Init starts the spinner, loads the initially selected environment, and arms auto-refresh.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, scheduleAutoRefresh(), func() tea.Msg {
		return initFetchMsg{}
	})
}

func scheduleAutoRefresh() tea.Cmd {
	return tea.Tick(autoRefreshInterval, func(time.Time) tea.Msg {
		return autoRefreshMsg{}
	})
}

func clampIndex(cur, length int) int {
	if length <= 0 {
		return 0
	}
	if cur < 0 {
		return 0
	}
	if cur >= length {
		return length - 1
	}
	return cur
}

func (m Model) openSelectedEnvTracked() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := m.selectedEnv()
	if !ok {
		return m, nil
	}
	if env.Namespace != "" {
		m.rightView = viewWorkloads
		m.namespace = env.Namespace
		m.loading = true
		return m.tracked(m.kubectlLineWorkloads(env.Namespace), false, func() tea.Msg {
			return m.fetchWorkloads(env, poolIdx, envIdx, env.Namespace)
		})
	}
	m.rightView = viewNamespaces
	m.loading = true
	return m.tracked(m.kubectlLineNamespaces(), false, func() tea.Msg {
		return m.fetchNamespaces(env, poolIdx, envIdx)
	})
}

func unavailableHint(env config.Environment) string {
	if env.Azure != nil && env.Azure.Subscription != "" {
		return fmt.Sprintf(
			"context %q not found in kubeconfig "+
				"(press a to az login, then l to ensure credentials)",
			env.Context,
		)
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
		return workloadsMsg{
			poolIdx: poolIdx, envIdx: envIdx, namespace: namespace,
			err: fmt.Errorf("%s", unavailableHint(env)),
		}
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
