package ui

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/elmosgot/kube-commander/internal/config"
	"github.com/elmosgot/kube-commander/internal/kube"
)

const logFollowFlushEvery = 100 * time.Millisecond

type logChunkMsg struct {
	id   int
	text string
	ch   <-chan tea.Msg
}

type logFollowDoneMsg struct {
	id  int
	err error
	ch  <-chan tea.Msg
}

func listenLogFollow(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return logFollowDoneMsg{}
		}
		return msg
	}
}

func (m *Model) stopLogFollow() {
	if m.logFollowCancel != nil {
		m.logFollowCancel()
		m.logFollowCancel = nil
	}
	m.logFollowID++
}

// startLogFollow cancels any prior stream and begins TailLines+Follow for the selected container.
func (m Model) startLogFollow() (Model, tea.Cmd) {
	env, _, _, ok := m.selectedEnv()
	if !ok || m.podDetail == nil {
		return m, nil
	}
	container := m.selectedContainerName()
	if container == "" {
		m.err = "no container selected"
		return m, nil
	}

	m.stopLogFollow()
	id := m.logFollowID
	ctx, cancel := context.WithCancel(context.Background())
	m.logFollowCancel = cancel

	m.rightView = viewPodLogs
	m.logText = ""
	m.logScroll = 0
	m.logStickBottom = true
	m.clearLogFilter()
	m.loading = true
	m.err = ""
	m.status = "following logs…"

	ns, pod := m.namespace, m.podName
	ch := make(chan tea.Msg, 32)
	go runLogFollow(ctx, env, ns, pod, container, id, ch)
	return m, tea.Batch(m.spinner.Tick, listenLogFollow(ch))
}

func runLogFollow(ctx context.Context, env config.Environment, namespace, pod, container string, id int, ch chan tea.Msg) {
	defer close(ch)

	client, err := kube.NewClient(env)
	if err != nil {
		ch <- logFollowDoneMsg{id: id, err: err, ch: ch}
		return
	}
	stream, err := client.FollowPodLogs(ctx, namespace, pod, container, 200)
	if err != nil {
		ch <- logFollowDoneMsg{id: id, err: err, ch: ch}
		return
	}
	defer stream.Close()

	reader := io.Reader(stream)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		text := buf.String()
		buf.Reset()
		select {
		case ch <- logChunkMsg{id: id, text: text, ch: ch}:
		case <-ctx.Done():
		}
	}

	ticker := time.NewTicker(logFollowFlushEvery)
	defer ticker.Stop()

	lines := make(chan string, 64)
	errCh := make(chan error, 1)
	go func() {
		defer close(lines)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
		}
	}()

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case line, ok := <-lines:
			if !ok {
				flush()
				var doneErr error
				select {
				case doneErr = <-errCh:
				default:
				}
				if ctx.Err() != nil {
					return
				}
				ch <- logFollowDoneMsg{id: id, err: doneErr, ch: ch}
				return
			}
			if buf.Len() > 0 {
				buf.WriteByte('\n')
			}
			buf.WriteString(line)
			if buf.Len() >= 32*1024 {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
