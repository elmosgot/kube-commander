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

func (model *Model) stopLogFollow() {
	if model.logFollowCmdID != 0 {
		model.finishCmd(model.logFollowCmdID, nil)
		model.logFollowCmdID = 0
	}
	if model.logFollowCancel != nil {
		model.logFollowCancel()
		model.logFollowCancel = nil
	}
	model.logFollowID++
}

// startLogFollow cancels any prior stream and begins TailLines+Follow for the selected container.
func (model Model) startLogFollow() (Model, tea.Cmd) {
	env, _, _, ok := model.selectedEnv()
	if !ok || model.podDetail == nil {
		return model, nil
	}
	container := model.selectedContainerName()
	if container == "" {
		model.err = "no container selected"
		return model, nil
	}

	model.stopLogFollow()
	followID := model.logFollowID
	ctx, cancel := context.WithCancel(context.Background())
	model.logFollowCancel = cancel

	model.rightView = viewPodLogs
	model.logText = ""
	model.logScroll = 0
	model.logStickBottom = true
	model.clearLogFilter()
	model.loading = true
	model.err = ""
	model.status = "following logs…"
	model.logFollowCmdID = model.beginCmd(model.kubectlLogsFollowLine(), true)

	ns, pod := model.namespace, model.podName
	msgCh := make(chan tea.Msg, 32)
	go runLogFollow(ctx, env, ns, pod, container, followID, msgCh)
	return model, tea.Batch(model.spinner.Tick, model.maybeCmdAnim(), listenLogFollow(msgCh))
}

func runLogFollow(
	ctx context.Context, env config.Environment, namespace, pod, container string, followID int, msgCh chan tea.Msg,
) {
	defer close(msgCh)

	stream, err := openLogStream(ctx, env, namespace, pod, container)
	if err != nil {
		msgCh <- logFollowDoneMsg{id: followID, err: err, ch: msgCh}
		return
	}
	defer func() { _ = stream.Close() }()

	scanner := bufio.NewScanner(io.Reader(stream))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines, errCh := scanLogLines(ctx, scanner)

	pumpLogLines(ctx, followID, msgCh, lines, errCh)
}

// openLogStream builds a client and starts the follow stream for one container.
func openLogStream(
	ctx context.Context, env config.Environment, namespace, pod, container string,
) (io.ReadCloser, error) {
	client, err := kube.NewClient(env)
	if err != nil {
		return nil, err
	}
	return client.FollowPodLogs(ctx, namespace, pod, container, 200)
}

// scanLogLines reads scanner lines into a channel until EOF or ctx cancellation.
func scanLogLines(ctx context.Context, scanner *bufio.Scanner) (<-chan string, <-chan error) {
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
	return lines, errCh
}

// pumpLogLines batches scanned lines and forwards flushed chunks to ch until done.
func pumpLogLines(ctx context.Context, followID int, msgCh chan tea.Msg, lines <-chan string, errCh <-chan error) {
	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		text := buf.String()
		buf.Reset()
		select {
		case msgCh <- logChunkMsg{id: followID, text: text, ch: msgCh}:
		case <-ctx.Done():
		}
	}

	ticker := time.NewTicker(logFollowFlushEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case line, ok := <-lines:
			if !ok {
				flush()
				finishLogFollow(ctx, followID, msgCh, errCh)
				return
			}
			appendLogLine(&buf, line, flush)
		case <-ticker.C:
			flush()
		}
	}
}

// appendLogLine adds line to buf, flushing early when the buffer grows large.
func appendLogLine(buf *strings.Builder, line string, flush func()) {
	if buf.Len() > 0 {
		buf.WriteByte('\n')
	}
	buf.WriteString(line)
	if buf.Len() >= 32*1024 {
		flush()
	}
}

// finishLogFollow emits the terminal done message unless ctx was cancelled.
func finishLogFollow(ctx context.Context, followID int, msgCh chan tea.Msg, errCh <-chan error) {
	var doneErr error
	select {
	case doneErr = <-errCh:
	default:
	}
	if ctx.Err() != nil {
		return
	}
	msgCh <- logFollowDoneMsg{id: followID, err: doneErr, ch: msgCh}
}
