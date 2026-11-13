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

func (m Model) paneWidths() (leftW, rightW int) {
	leftW = m.width / 3
	if leftW < 24 {
		leftW = 24
	}
	if leftW > 40 {
		leftW = 40
	}
	rightW = m.width - leftW - 1
	if rightW < 20 {
		rightW = 20
	}
	return leftW, rightW
}

// View renders the two panes and footer.
func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	bodyH, cmdsH := layoutHeights(m.height, m.showCmds)
	leftW, rightW := m.paneWidths()

	left := m.renderLeft(leftW, bodyH)
	right := m.renderRight(rightW, bodyH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	header := titleStyle.Render("kube-commander") + "  " + mutedStyle.Render(truncate(m.configPath, m.width-20))
	footer := m.renderFooter()
	help := m.renderHelp()

	parts := []string{header, body}
	if m.showCmds && cmdsH > 0 {
		parts = append(parts, m.renderCmdsPanel(m.width, cmdsH))
	}
	parts = append(parts, footer, help)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderHelp() string {
	if m.rightView == viewPodLogs && m.logFilterActive {
		line := "Filter: " + m.logFilter + "█  ·  ! exclude · | or · esc clear · ↑↓ scroll"
		return helpStyle.Render(truncate(line, m.width))
	}

	parts := []string{"↑↓ move", "tab focus"}
	if m.focus == paneLeft {
		parts = append(parts, "enter open", helpAzLogin, "l ensure", "c commands", "r refresh", "q quit")
	} else {
		switch m.rightView {
		case viewPodDetail:
			parts = append(parts, helpLogs, "s shell", helpBackspaceBack, "c commands", "r refresh", "q quit")
		case viewPodLogs:
			parts = append(parts,
				helpFilter, "y copy", "ctrl+click copy", "r restart follow",
				helpBackspaceBack, "c commands", "q quit",
			)
		default:
			parts = append(parts, "enter open", helpBackspaceBack, "c commands", "r refresh", "q quit")
		}
	}
	return helpStyle.Render(truncate(strings.Join(parts, " · "), m.width))
}

func (m Model) renderCmdsPanel(width, height int) string {
	inner := height - 2 // border
	if inner < 1 {
		inner = 1
	}
	now := time.Now()
	rows := buildCmdPanelRows(m.cmdLog, m.cmdAnimFrame, now, width)
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
	var b strings.Builder
	dotBright := m.cmdAnimFrame%2 == 0
	for _, row := range rows {
		switch row.kind {
		case rowKindTitle:
			b.WriteString(sectionStyle.Render(row.text) + "\n")
		case rowKindErr:
			b.WriteString(errorStyle.Render("  "+row.text) + "\n")
		default:
			m.writeCmdRow(&b, row, dotBright)
		}
	}
	if len(m.cmdLog) == 0 {
		b.WriteString(cmdsStyle.Render("(no commands yet)") + "\n")
	}
	return fitBox(paneStyle, width, height, b.String())
}

func (m Model) writeCmdRow(b *strings.Builder, row cmdPanelRow, dotBright bool) {
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
	b.WriteString(prefix + cmdsStyle.Render(cmdPart+" "+timing) + "\n")
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
		style := m.leftRowStyle(i, row)
		b.WriteString(style.Width(width - 4).Render(truncate(line, width-4)))
		b.WriteByte('\n')
		written++
	}
	return fitBox(border, width, height, b.String())
}

// leftRowStyle picks the style for a left-pane row based on cursor/focus/availability.
func (m Model) leftRowStyle(i int, row leftRow) lipgloss.Style {
	if i == m.leftCursor {
		if m.focus == paneLeft {
			return selectedStyle
		}
		return selectedDimStyle
	}
	if !row.isPool {
		env := m.cfg.Pools[row.poolIdx].Environments[row.envIdx]
		if !m.available[envKey(env)] {
			return unavailableStyle
		}
	}
	return itemStyle
}

func (m Model) renderRight(width, height int) string {
	border := paneStyle
	if m.focus == paneRight {
		border = paneFocusedStyle
	}

	var b strings.Builder
	b.WriteString(sectionStyle.Render(m.rightTitle()) + "\n")

	switch {
	case m.loading:
		b.WriteString(m.spinner.View() + " loading…\n")
	case m.err != "" && m.rightView != viewPodLogs && m.rightLen() == 0:
		b.WriteString(errorStyle.Render(truncate(m.err, width-4)) + "\n")
	default:
		m.renderRightBody(&b, width, height)
	}

	return fitBox(border, width, height, b.String())
}

func (m Model) rightTitle() string {
	switch m.rightView {
	case viewWorkloads:
		return fmt.Sprintf("Workloads · %s", m.namespace)
	case viewPods:
		return fmt.Sprintf("Pods · %s", m.deployment)
	case viewPodDetail:
		return fmt.Sprintf("Pod · %s", m.podName)
	case viewPodLogs:
		return fmt.Sprintf("Logs · %s / %s", m.podName, m.selectedContainerName())
	default:
		return "Namespaces"
	}
}

func (m Model) renderRightBody(b *strings.Builder, width, height int) {
	switch m.rightView {
	case viewNamespaces:
		m.renderNamespaceRows(b, width)
	case viewWorkloads:
		m.renderWorkloadRows(b, width)
	case viewPods:
		m.renderPodRows(b, width)
	case viewPodDetail:
		m.renderPodDetail(b, width)
	case viewPodLogs:
		m.renderPodLogs(b, width, height)
	}
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
		cpu := p.CPURequest + "/" + p.CPULimit
		mem := p.MemRequest + "/" + p.MemLimit
		line := fmt.Sprintf("%-28s  %-5s  r %-3d  %-10s  cpu %-12s  mem %-14s  %s",
			p.Name, p.Ready, p.Restarts, p.Status, cpu, mem, p.Age)
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
	fmt.Fprintf(b, "phase=%s  ready=%s  node=%s  ip=%s  age=%s\n",
		d.Phase, d.Ready, d.Node, d.PodIP, d.Age)
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
	logical := m.filteredLogLines()
	if m.logsEmpty(logical) {
		if m.logFilterActive && m.logFilter != "" {
			b.WriteString(mutedStyle.Render("no matching lines") + "\n")
			return
		}
		b.WriteString(mutedStyle.Render("no log output") + "\n")
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
	start, end := logWindow(len(rows), m.logScroll, visible)
	for _, row := range rows[start:end] {
		b.WriteString(itemStyle.Width(contentW).Render(row.text))
		b.WriteByte('\n')
	}
}

// logsEmpty reports whether the filtered log slice has nothing worth rendering.
func (m Model) logsEmpty(logical []string) bool {
	if len(logical) == 0 {
		return true
	}
	return len(logical) == 1 && logical[0] == "" && (!m.logFilterActive || m.logFilter == "")
}

// logWindow clamps scroll to a valid [start,end) slice window over n rows.
func logWindow(n, scroll, visible int) (start, end int) {
	maxStart := n - visible
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
	if end > n {
		end = n
	}
	return start, end
}

func (m Model) renderFooter() string {
	line := strings.Join(m.footerParts(), "  ·  ")
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

// footerParts builds the breadcrumb segments (pool/env/namespace/pod/…) for the footer.
func (m Model) footerParts() []string {
	parts := []string{}
	if pool := m.selectedPoolName(); pool != "" {
		parts = append(parts, "pool="+pool)
	}
	env, _, _, ok := m.selectedEnv()
	if !ok {
		return append(parts, "select an environment")
	}
	parts = append(parts, "env="+env.Name, "context="+env.Context)
	if ns := m.footerNamespace(env); ns != "" {
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
	return append(parts, "kubeconfig="+config.ResolveKubeconfig(env))
}

func (m Model) footerNamespace(env config.Environment) string {
	if m.namespace != "" {
		return m.namespace
	}
	return env.Namespace
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
