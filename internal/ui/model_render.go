package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/elmosgot/kube-commander/internal/config"
)

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

func (model Model) paneWidths() (leftW, rightW int) {
	leftW = model.width / 3
	if leftW < 24 {
		leftW = 24
	}
	if leftW > 40 {
		leftW = 40
	}
	rightW = model.width - leftW - 1
	if rightW < 20 {
		rightW = 20
	}
	return leftW, rightW
}

// View renders the two panes and footer.
func (model Model) View() string {
	if model.width == 0 {
		return "loading..."
	}

	bodyH, cmdsH := layoutHeights(model.height, model.showCmds)
	leftW, rightW := model.paneWidths()

	left := model.renderLeft(leftW, bodyH)
	right := model.renderRight(rightW, bodyH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	header := titleStyle.Render("kube-commander") + "  " + mutedStyle.Render(truncate(model.configPath, model.width-20))
	footer := model.renderFooter()
	help := model.renderHelp()

	parts := []string{header, body}
	if model.showCmds && cmdsH > 0 {
		parts = append(parts, model.renderCmdsPanel(model.width, cmdsH))
	}
	parts = append(parts, footer, help)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (model Model) renderHelp() string {
	if model.rightView == viewPodLogs && model.logFilterActive {
		line := "Filter: " + model.logFilter + "█  ·  ! exclude · | or · esc clear · ↑↓ scroll"
		return helpStyle.Render(truncate(line, model.width))
	}

	parts := []string{"↑↓ move", "tab focus"}
	if model.focus == paneLeft {
		parts = append(parts, helpEnterOpen, helpAzLogin, helpEnsure, "c commands", "r refresh", "q quit")
	} else {
		switch model.rightView {
		case viewPodDetail:
			parts = append(parts, helpLogs, helpShell, helpBackspaceBack, "c commands", "r refresh", "q quit")
		case viewPodLogs:
			parts = append(parts,
				helpFilter, "y copy", "ctrl+click copy", "r restart follow",
				helpBackspaceBack, "c commands", "q quit",
			)
		default:
			parts = append(parts, helpEnterOpen, helpBackspaceBack, "c commands", "r refresh", "q quit")
		}
	}
	return helpStyle.Render(truncate(strings.Join(parts, " · "), model.width))
}

func (model Model) renderCmdsPanel(width, height int) string {
	inner := height - 2 // border
	if inner < 1 {
		inner = 1
	}
	now := time.Now()
	rows := buildCmdPanelRows(model.cmdLog, model.cmdAnimFrame, now, width)
	// Show newest entries that fit (title + tail).
	if len(rows) > inner {
		keep := inner - 1
		if keep < 0 {
			keep = 0
		}
		tail := rows[1:]
		if len(tail) > keep {
			tail = tail[len(tail)-keep:]
		}
		rows = append([]cmdPanelRow{rows[0]}, tail...)
	}
	var buf strings.Builder
	dotBright := model.cmdAnimFrame%2 == 0
	for _, row := range rows {
		switch row.kind {
		case rowKindTitle:
			buf.WriteString(sectionStyle.Render(row.text) + "\n")
		case rowKindErr:
			buf.WriteString(errorStyle.Render("  "+row.text) + "\n")
		default:
			model.writeCmdRow(&buf, row, dotBright)
		}
	}
	if len(model.cmdLog) == 0 {
		buf.WriteString(cmdsStyle.Render("(no commands yet)") + "\n")
	}
	return fitBox(paneStyle, width, height, buf.String())
}

func (model Model) writeCmdRow(buf *strings.Builder, row cmdPanelRow, dotBright bool) {
	cmdCols := row.cmdCols
	if cmdCols < 8 {
		cmdCols = 8
	}
	cmdPart := padRightRunes(row.text, cmdCols)
	timing := padLeftRunes(row.timing, cmdTimingColWidth)
	var prefix string
	if row.running {
		dot := cmdsDotDim
		if dotBright {
			dot = cmdsDotBright
		}
		prefix = dot.Render("●") + " "
	} else {
		prefix = cmdsStyle.Render("  ")
	}
	buf.WriteString(prefix + cmdsStyle.Render(cmdPart+" "+timing) + "\n")
}

func fitBox(style lipgloss.Style, width, height int, content string) string {
	return style.Width(width).Height(height).MaxWidth(width).MaxHeight(height).Render(content)
}

func (model Model) renderLeft(width, height int) string {
	border := paneStyle
	if model.focus == paneLeft {
		border = paneFocusedStyle
	}
	var buf strings.Builder
	buf.WriteString(sectionStyle.Render("Pools") + "\n")
	innerRows := height - 2 // border top/bottom
	if innerRows < 1 {
		innerRows = 1
	}
	written := 1 // title
	for i, row := range model.leftRows {
		if written >= innerRows {
			break
		}
		line := row.label
		if row.isPool {
			marker := "▾"
			if !model.expanded[row.poolIdx] {
				marker = "▸"
			}
			line = marker + " " + row.label
		}
		style := model.leftRowStyle(i, row)
		buf.WriteString(style.Width(width - 4).Render(truncate(line, width-4)))
		buf.WriteByte('\n')
		written++
	}
	return fitBox(border, width, height, buf.String())
}

// leftRowStyle picks the style for a left-pane row based on cursor/focus/availability.
func (model Model) leftRowStyle(i int, row leftRow) lipgloss.Style {
	if i == model.leftCursor {
		if model.focus == paneLeft {
			return selectedStyle
		}
		return selectedDimStyle
	}
	if !row.isPool {
		env := model.cfg.Pools[row.poolIdx].Environments[row.envIdx]
		if !model.available[envKey(env)] {
			return unavailableStyle
		}
	}
	return itemStyle
}

func (model Model) renderRight(width, height int) string {
	border := paneStyle
	if model.focus == paneRight {
		border = paneFocusedStyle
	}

	var buf strings.Builder
	buf.WriteString(sectionStyle.Render(model.rightTitle()) + "\n")

	switch {
	case model.loading:
		buf.WriteString(model.spinner.View() + " loading…\n")
	case model.err != "" && model.rightView != viewPodLogs && model.rightLen() == 0:
		buf.WriteString(errorStyle.Render(truncate(model.err, width-4)) + "\n")
	default:
		model.renderRightBody(&buf, width, height)
	}

	return fitBox(border, width, height, buf.String())
}

func (model Model) rightTitle() string {
	switch model.rightView {
	case viewWorkloads:
		return fmt.Sprintf("Workloads · %s", model.namespace)
	case viewPods:
		return fmt.Sprintf("Pods · %s", model.deployment)
	case viewPodDetail:
		return fmt.Sprintf("Pod · %s", model.podName)
	case viewPodLogs:
		return fmt.Sprintf("Logs · %s / %s", model.podName, model.selectedContainerName())
	default:
		return "Namespaces"
	}
}

func (model Model) renderRightBody(buf *strings.Builder, width, height int) {
	switch model.rightView {
	case viewNamespaces:
		model.renderNamespaceRows(buf, width)
	case viewWorkloads:
		model.renderWorkloadRows(buf, width)
	case viewPods:
		model.renderPodRows(buf, width)
	case viewPodDetail:
		model.renderPodDetail(buf, width)
	case viewPodLogs:
		model.renderPodLogs(buf, width, height)
	}
}

func (model Model) rowStyle(selected bool) lipgloss.Style {
	if !selected {
		return itemStyle
	}
	if model.focus == paneRight {
		return selectedStyle
	}
	return selectedDimStyle
}

func (model Model) renderNamespaceRows(buf *strings.Builder, width int) {
	if len(model.namespaces) == 0 {
		buf.WriteString(mutedStyle.Render("no namespaces") + "\n")
		return
	}
	for i, ns := range model.namespaces {
		line := fmt.Sprintf("%-24s  deps %-3d  pods %d/%d",
			ns.Name, ns.DeploymentCount, ns.ReadyPods, ns.TotalPods)
		buf.WriteString(model.rowStyle(i == model.rightCursor).Width(width - 4).Render(truncate(line, width-4)))
		buf.WriteByte('\n')
	}
}

func (model Model) renderWorkloadRows(buf *strings.Builder, width int) {
	if len(model.workloads) == 0 {
		buf.WriteString(mutedStyle.Render("no deployments") + "\n")
		return
	}
	for i, w := range model.workloads {
		line := fmt.Sprintf("%-28s  %d/%d  age %s", w.Name, w.Ready, w.Desired, w.Age)
		buf.WriteString(model.rowStyle(i == model.rightCursor).Width(width - 4).Render(truncate(line, width-4)))
		buf.WriteByte('\n')
	}
}

func (model Model) renderPodRows(buf *strings.Builder, width int) {
	if len(model.pods) == 0 {
		buf.WriteString(mutedStyle.Render("no pods") + "\n")
		return
	}
	for i, p := range model.pods {
		cpu := p.CPURequest + "/" + p.CPULimit
		mem := p.MemRequest + "/" + p.MemLimit
		line := fmt.Sprintf("%-28s  %-5s  r %-3d  %-10s  cpu %-12s  mem %-14s  %s",
			p.Name, p.Ready, p.Restarts, p.Status, cpu, mem, p.Age)
		buf.WriteString(model.rowStyle(i == model.rightCursor).Width(width - 4).Render(truncate(line, width-4)))
		buf.WriteByte('\n')
	}
}

func (model Model) renderPodDetail(buf *strings.Builder, width int) {
	if model.podDetail == nil {
		buf.WriteString(mutedStyle.Render("no pod detail") + "\n")
		return
	}
	d := model.podDetail
	fmt.Fprintf(buf, "phase=%s  ready=%s  node=%s  ip=%s  age=%s\n",
		d.Phase, d.Ready, d.Node, d.PodIP, d.Age)
	buf.WriteString(mutedStyle.Render("containers (enter/g = logs)") + "\n")
	for i, ctr := range d.Containers {
		ready := "no"
		if ctr.Ready {
			ready = "yes"
		}
		line := fmt.Sprintf("%-20s  %-7s  rst %-3d  %-12s  cpu %s/%s  mem %s/%s",
			ctr.Name, ready, ctr.Restarts, ctr.State, ctr.CPURequest, ctr.CPULimit, ctr.MemRequest, ctr.MemLimit)
		buf.WriteString(model.rowStyle(i == model.containerIdx).Width(width - 4).Render(truncate(line, width-4)))
		buf.WriteByte('\n')
		img := "  image " + ctr.Image
		buf.WriteString(mutedStyle.Width(width - 4).Render(truncate(img, width-4)))
		buf.WriteByte('\n')
	}
}

func (model Model) renderPodLogs(buf *strings.Builder, width, height int) {
	if model.err != "" && model.logText == "" {
		buf.WriteString(errorStyle.Render(truncate(model.err, width-4)) + "\n")
		return
	}
	logical := model.filteredLogLines()
	if model.logsEmpty(logical) {
		if model.logFilterActive && model.logFilter != "" {
			buf.WriteString(mutedStyle.Render("no matching lines") + "\n")
			return
		}
		buf.WriteString(mutedStyle.Render("no log output") + "\n")
		return
	}
	contentW := width - 4
	if contentW < 1 {
		contentW = 1
	}
	rows := buildVisualLogRows(logical, contentW)
	visible := height - 4
	if visible < 3 {
		visible = 3
	}
	start, end := logWindow(len(rows), model.logScroll, visible)
	for _, row := range rows[start:end] {
		buf.WriteString(itemStyle.Width(contentW).Render(row.text))
		buf.WriteByte('\n')
	}
}

// logsEmpty reports whether the filtered log slice has nothing worth rendering.
func (model Model) logsEmpty(logical []string) bool {
	if len(logical) == 0 {
		return true
	}
	return len(logical) == 1 && logical[0] == "" && (!model.logFilterActive || model.logFilter == "")
}

// logWindow clamps scroll to a valid [start,end) slice window over total rows.
func logWindow(total, scroll, visible int) (start, end int) {
	maxStart := total - visible
	if maxStart < 0 {
		maxStart = 0
	}
	start = scroll
	if start < 0 {
		start = 0
	}
	if start > maxStart {
		start = maxStart
	}
	end = start + visible
	if end > total {
		end = total
	}
	return start, end
}

func (model Model) renderFooter() string {
	line := strings.Join(model.footerParts(), "  ·  ")
	if model.status != "" {
		line += "  ·  " + model.status
	}
	if model.err != "" {
		line += "  ·  " + model.err
	}
	// Single visual row so the height budget stays valid.
	w := model.width
	if w <= 0 {
		w = 80
	}
	line = truncate(line, w)
	if model.err != "" && strings.Contains(line, model.err) {
		idx := strings.LastIndex(line, model.err)
		return footerStyle.Render(line[:idx]) + errorStyle.Render(line[idx:])
	}
	return footerStyle.Render(line)
}

// footerParts builds the breadcrumb segments (pool/env/namespace/pod/…) for the footer.
func (model Model) footerParts() []string {
	parts := []string{}
	if pool := model.selectedPoolName(); pool != "" {
		parts = append(parts, "pool="+pool)
	}
	env, _, _, ok := model.selectedEnv()
	if !ok {
		return append(parts, "select an environment")
	}
	parts = append(parts, "env="+env.Name, "context="+env.Context)
	if ns := model.footerNamespace(env); ns != "" {
		parts = append(parts, "namespace="+ns)
	}
	if model.deployment != "" {
		parts = append(parts, "deploy="+model.deployment)
	}
	if model.podName != "" {
		parts = append(parts, "pod="+model.podName)
	}
	if ctr := model.selectedContainerName(); ctr != "" &&
		(model.rightView == viewPodDetail || model.rightView == viewPodLogs) {
		parts = append(parts, "container="+ctr)
	}
	return append(parts, "kubeconfig="+config.ResolveKubeconfig(env))
}

func (model Model) footerNamespace(env config.Environment) string {
	if model.namespace != "" {
		return model.namespace
	}
	return env.Namespace
}

func truncate(text string, count int) string {
	if count <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= count {
		return text
	}
	if count <= 1 {
		return string(runes[:count])
	}
	return string(runes[:count-1]) + "…"
}

var (
	titleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	mutedStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	footerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	sectionStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	cmdsStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cmdsDotBright    = lipgloss.NewStyle().Foreground(lipgloss.Color("65"))  // soft green
	cmdsDotDim       = lipgloss.NewStyle().Foreground(lipgloss.Color("238")) // near-muted
	itemStyle        = lipgloss.NewStyle()
	selectedStyle    = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
	selectedDimStyle = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
	unavailableStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Italic(true)
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	paneStyle        = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("238")).
				Padding(0, 1)
	paneFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("62")).
				Padding(0, 1)
)
