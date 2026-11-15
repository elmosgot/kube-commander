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
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	model := Model{
		cfg:        cfg,
		configPath: configPath,
		focus:      paneLeft,
		expanded:   map[int]bool{},
		available:  map[string]bool{},
		spinner:    spin,
	}
	for i := range cfg.Pools {
		model.expanded[i] = true
	}
	model.refreshAvailability()
	model.rebuildLeft()
	if len(model.leftRows) > 0 {
		for i, row := range model.leftRows {
			if !row.isPool {
				model.leftCursor = i
				break
			}
		}
	}
	return model
}

func envKey(env config.Environment) string {
	return config.ResolveKubeconfig(env) + "|" + env.Context
}

func (model *Model) refreshAvailability() {
	for i := range model.cfg.Pools {
		for _, env := range model.cfg.Pools[i].Environments {
			model.available[envKey(env)] = kube.ContextExists(env)
		}
	}
}

func (model *Model) rebuildLeft() {
	rows := make([]leftRow, 0)
	for i, pool := range model.cfg.Pools {
		rows = append(rows, leftRow{
			poolIdx: i,
			envIdx:  -1,
			label:   pool.Name,
			isPool:  true,
		})
		if !model.expanded[i] {
			continue
		}
		for j, env := range pool.Environments {
			mark := ""
			if !model.available[envKey(env)] {
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
	model.leftRows = rows
	if model.leftCursor >= len(model.leftRows) {
		model.leftCursor = len(model.leftRows) - 1
	}
	if model.leftCursor < 0 {
		model.leftCursor = 0
	}
}

func (model Model) selectedEnv() (config.Environment, int, int, bool) {
	if len(model.leftRows) == 0 {
		return config.Environment{}, 0, 0, false
	}
	row := model.leftRows[model.leftCursor]
	if row.isPool || row.envIdx < 0 {
		return config.Environment{}, row.poolIdx, -1, false
	}
	env := model.cfg.Pools[row.poolIdx].Environments[row.envIdx]
	return env, row.poolIdx, row.envIdx, true
}

func (model Model) selectedPoolName() string {
	if len(model.leftRows) == 0 {
		return ""
	}
	row := model.leftRows[model.leftCursor]
	return model.cfg.Pools[row.poolIdx].Name
}

func (model Model) selectedContainerName() string {
	if model.podDetail == nil || len(model.podDetail.Containers) == 0 {
		return ""
	}
	if model.containerIdx < 0 || model.containerIdx >= len(model.podDetail.Containers) {
		return model.podDetail.Containers[0].Name
	}
	return model.podDetail.Containers[model.containerIdx].Name
}

type initFetchMsg struct{}

// Init starts the spinner, loads the initially selected environment, and arms auto-refresh.
func (model Model) Init() tea.Cmd {
	return tea.Batch(model.spinner.Tick, scheduleAutoRefresh(), func() tea.Msg {
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

func (model Model) openSelectedEnvTracked() (tea.Model, tea.Cmd) {
	env, poolIdx, envIdx, ok := model.selectedEnv()
	if !ok {
		return model, nil
	}
	if env.Namespace != "" {
		model.rightView = viewWorkloads
		model.namespace = env.Namespace
		model.loading = true
		return model.tracked(model.kubectlLineWorkloads(env.Namespace), false, func() tea.Msg {
			return model.fetchWorkloads(env, poolIdx, envIdx, env.Namespace)
		})
	}
	model.rightView = viewNamespaces
	model.loading = true
	return model.tracked(model.kubectlLineNamespaces(), false, func() tea.Msg {
		return model.fetchNamespaces(env, poolIdx, envIdx)
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

func (model Model) fetchNamespaces(env config.Environment, poolIdx, envIdx int) tea.Msg {
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

func (model Model) fetchWorkloads(env config.Environment, poolIdx, envIdx int, namespace string) tea.Msg {
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

func (model Model) fetchPods(env config.Environment, poolIdx, envIdx int, namespace, deployment string) tea.Msg {
	client, err := kube.NewClient(env)
	if err != nil {
		return podsMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, deployment: deployment, err: err}
	}
	items, err := client.ListPods(context.Background(), namespace, deployment)
	return podsMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, deployment: deployment, items: items, err: err}
}

func (model Model) fetchPodDetail(env config.Environment, poolIdx, envIdx int, namespace, pod string) tea.Msg {
	client, err := kube.NewClient(env)
	if err != nil {
		return podDetailMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, pod: pod, err: err}
	}
	detail, err := client.GetPod(context.Background(), namespace, pod)
	return podDetailMsg{poolIdx: poolIdx, envIdx: envIdx, namespace: namespace, pod: pod, detail: detail, err: err}
}
