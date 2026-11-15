package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	maxCmdLogEntries = 30
	cmdAnimInterval  = 450 * time.Millisecond
)

// Command-panel row kinds (used for rendering and click hit-testing).
const (
	rowKindTitle = "title"
	rowKindCmd   = "cmd"
	rowKindErr   = "err"
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

func (model *Model) beginCmd(line string, infinite bool) int {
	model.cmdLogSeq++
	cmdID := model.cmdLogSeq
	model.cmdLog = append(model.cmdLog, cmdEntry{
		ID:       cmdID,
		Line:     line,
		Started:  time.Now(),
		Running:  true,
		Infinite: infinite,
	})
	if len(model.cmdLog) > maxCmdLogEntries {
		model.cmdLog = model.cmdLog[len(model.cmdLog)-maxCmdLogEntries:]
	}
	return cmdID
}

func (model *Model) finishCmd(id int, err error) {
	for i := range model.cmdLog {
		if model.cmdLog[i].ID != id {
			continue
		}
		model.cmdLog[i].Running = false
		model.cmdLog[i].Ended = time.Now()
		if err != nil {
			model.cmdLog[i].Err = err.Error()
		}
		return
	}
}

func (model Model) runningCmdCount() int {
	count := 0
	for _, entry := range model.cmdLog {
		if entry.Running {
			count++
		}
	}
	return count
}

func (model Model) maybeCmdAnim() tea.Cmd {
	if model.showCmds && model.runningCmdCount() > 0 {
		return scheduleCmdAnim()
	}
	return nil
}

// tracked starts a cmd log entry then runs work, finishing the entry when work returns.
func (model Model) tracked(line string, infinite bool, work func() tea.Msg) (Model, tea.Cmd) {
	id := model.beginCmd(line, infinite)
	return model, tea.Batch(model.maybeCmdAnim(), func() tea.Msg {
		inner := work()
		return cmdDoneMsg{id: id, err: errFromMsg(inner), inner: inner}
	})
}

func errFromMsg(msg tea.Msg) error {
	switch message := msg.(type) {
	case namespacesMsg:
		return message.err
	case workloadsMsg:
		return message.err
	case podsMsg:
		return message.err
	case podDetailMsg:
		return message.err
	case ensureMsg:
		return message.err
	case loginMsg:
		return message.err
	case execMsg:
		return message.err
	default:
		return nil
	}
}

func formatCmdDuration(dur time.Duration) string {
	if dur < time.Second {
		return fmt.Sprintf("%dms", dur.Milliseconds())
	}
	if dur < time.Minute {
		return fmt.Sprintf("%.1fs", dur.Seconds())
	}
	return fmt.Sprintf("%.1fm", dur.Minutes())
}

func (entry cmdEntry) durationLabel(now time.Time) string {
	if entry.Running && entry.Infinite {
		return "∞"
	}
	end := entry.Ended
	if entry.Running {
		end = now
	}
	if end.IsZero() {
		end = now
	}
	return formatCmdDuration(end.Sub(entry.Started))
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
	rows := []cmdPanelRow{{kind: rowKindTitle, text: "Commands"}}
	inner := width - 4
	if inner < 12 {
		inner = 12
	}
	// "● "/"  " + command + " " + timing column
	cmdCols := inner - 2 - 1 - cmdTimingColWidth
	if cmdCols < 8 {
		cmdCols = 8
	}
	for _, entry := range entries {
		line := truncate(entry.Line, cmdCols)
		timing := entry.durationLabel(now)
		if entry.Running && entry.Infinite {
			timing = "∞"
		}
		rows = append(rows, cmdPanelRow{
			kind:    rowKindCmd,
			entryID: entry.ID,
			text:    line,
			timing:  timing,
			running: entry.Running,
			cmdCols: cmdCols,
		})
		if entry.Err != "" && !entry.Running {
			errText := entry.Err
			if width > 4 {
				errText = truncate(errText, width-4)
			}
			rows = append(rows, cmdPanelRow{kind: rowKindErr, entryID: entry.ID, text: errText})
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
