package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	maxCmdLogEntries   = 30
	cmdAnimInterval    = 450 * time.Millisecond
)

type cmdEntry struct {
	ID       int
	Line     string
	Started  time.Time
	Ended    time.Time
	Running  bool
	Infinite bool
	Err      string
}

type cmdDoneMsg struct {
	id    int
	err   error
	inner tea.Msg
}

type cmdAnimMsg struct{}

func scheduleCmdAnim() tea.Cmd {
	return tea.Tick(cmdAnimInterval, func(time.Time) tea.Msg {
		return cmdAnimMsg{}
	})
}

func (m *Model) beginCmd(line string, infinite bool) int {
	m.cmdLogSeq++
	id := m.cmdLogSeq
	m.cmdLog = append(m.cmdLog, cmdEntry{
		ID:       id,
		Line:     line,
		Started:  time.Now(),
		Running:  true,
		Infinite: infinite,
	})
	if len(m.cmdLog) > maxCmdLogEntries {
		m.cmdLog = m.cmdLog[len(m.cmdLog)-maxCmdLogEntries:]
	}
	return id
}

func (m *Model) finishCmd(id int, err error) {
	for i := range m.cmdLog {
		if m.cmdLog[i].ID != id {
			continue
		}
		m.cmdLog[i].Running = false
		m.cmdLog[i].Ended = time.Now()
		if err != nil {
			m.cmdLog[i].Err = err.Error()
		}
		return
	}
}

func (m Model) runningCmdCount() int {
	n := 0
	for _, e := range m.cmdLog {
		if e.Running {
			n++
		}
	}
	return n
}

func (m Model) maybeCmdAnim() tea.Cmd {
	if m.showCmds && m.runningCmdCount() > 0 {
		return scheduleCmdAnim()
	}
	return nil
}

// tracked starts a cmd log entry then runs work, finishing the entry when work returns.
func (m Model) tracked(line string, infinite bool, work func() tea.Msg) (Model, tea.Cmd) {
	id := m.beginCmd(line, infinite)
	return m, tea.Batch(m.maybeCmdAnim(), func() tea.Msg {
		inner := work()
		return cmdDoneMsg{id: id, err: errFromMsg(inner), inner: inner}
	})
}

func errFromMsg(msg tea.Msg) error {
	switch m := msg.(type) {
	case namespacesMsg:
		return m.err
	case workloadsMsg:
		return m.err
	case podsMsg:
		return m.err
	case podDetailMsg:
		return m.err
	case ensureMsg:
		return m.err
	case loginMsg:
		return m.err
	case execMsg:
		return m.err
	default:
		return nil
	}
}

func formatCmdDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

func (e cmdEntry) durationLabel(now time.Time) string {
	if e.Running && e.Infinite {
		return "∞"
	}
	end := e.Ended
	if e.Running {
		end = now
	}
	if end.IsZero() {
		end = now
	}
	return formatCmdDuration(end.Sub(e.Started))
}

const cmdTimingColWidth = 8 // right-aligned duration / ∞ column

// cmdPanelRow is one rendered row in the commands panel (for hit-testing).
type cmdPanelRow struct {
	kind    string // "title" | "cmd" | "err"
	entryID int
	text    string // command only (no timing)
	timing  string // duration or ∞, shown in a fixed right column
	running bool
	cmdCols int // rune width reserved for the command column
}

func buildCmdPanelRows(entries []cmdEntry, animFrame int, now time.Time, width int) []cmdPanelRow {
	rows := []cmdPanelRow{{kind: "title", text: "Commands"}}
	inner := width - 4
	if inner < 12 {
		inner = 12
	}
	// "● "/"  " + command + " " + timing column
	cmdCols := inner - 2 - 1 - cmdTimingColWidth
	if cmdCols < 8 {
		cmdCols = 8
	}
	for _, e := range entries {
		line := truncate(e.Line, cmdCols)
		timing := e.durationLabel(now)
		if e.Running && e.Infinite {
			timing = "∞"
		}
		rows = append(rows, cmdPanelRow{
			kind:    "cmd",
			entryID: e.ID,
			text:    line,
			timing:  timing,
			running: e.Running,
			cmdCols: cmdCols,
		})
		if e.Err != "" && !e.Running {
			errText := e.Err
			if width > 4 {
				errText = truncate(errText, width-4)
			}
			rows = append(rows, cmdPanelRow{kind: "err", entryID: e.ID, text: errText})
		}
	}
	_ = animFrame
	return rows
}

func padRightRunes(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[:n])
	}
	return s + strings.Repeat(" ", n-len(r))
}

func padLeftRunes(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[len(r)-n:])
	}
	return strings.Repeat(" ", n-len(r)) + s
}

func cmdEntryByID(entries []cmdEntry, id int) (cmdEntry, bool) {
	for _, e := range entries {
		if e.ID == id {
			return e, true
		}
	}
	return cmdEntry{}, false
}
