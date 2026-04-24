package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/matt/sysiphus/internal/state"
)

func (m *model) renderHeader() string {
	left := m.styles.title.Render("sysiphus")
	meta := []string{
		fmt.Sprintf("mode %s", m.mode),
		fmt.Sprintf("policy %s", m.policy),
		fmt.Sprintf("model %s", m.modelName),
		fmt.Sprintf("status %s", m.status),
	}
	if m.branch != "" {
		meta = append(meta, fmt.Sprintf("branch %s", m.branch))
	}
	if m.sessionID != "" {
		meta = append(meta, fmt.Sprintf("session %s", shortID(m.sessionID)))
	}
	contentWidth := max(1, m.width-2)
	rightText := truncate(strings.Join(meta, "  "), max(1, contentWidth-lipgloss.Width(left)-1))
	gapWidth := max(1, contentWidth-lipgloss.Width(left)-lipgloss.Width(rightText))
	right := m.styles.muted.Render(rightText)
	line := left + strings.Repeat(" ", gapWidth) + right
	return lipgloss.NewStyle().Padding(0, 1).Render(line)
}

func (m *model) layoutMetrics() (bodyHeight, leftWidth, rightWidth, rightTopHeight, rightBottomHeight int) {
	headerHeight := lipgloss.Height(m.renderHeader())
	footerHeight := lipgloss.Height(m.renderFooter())
	helpHeight := 0
	if m.showHelp {
		helpHeight = lipgloss.Height(m.styles.help.Render(helpText()))
	}
	bodyHeight = m.height - headerHeight - footerHeight - helpHeight
	if bodyHeight < 0 {
		bodyHeight = 0
	}
	if m.width <= 0 || bodyHeight <= 0 {
		return
	}
	const colGap = 1
	leftWidth = int(float64(m.width) * 0.62)
	if leftWidth < 40 {
		leftWidth = 40
	}
	if leftWidth > m.width-colGap-24 {
		leftWidth = max(1, m.width-colGap-24)
	}
	rightWidth = max(1, m.width-leftWidth-colGap)

	rightTopHeight = max(5, bodyHeight/3)
	if rightTopHeight > bodyHeight-5 {
		rightTopHeight = max(3, bodyHeight-5)
	}
	rightBottomHeight = bodyHeight - rightTopHeight
	if rightBottomHeight < 3 {
		rightBottomHeight = 3
		rightTopHeight = max(3, bodyHeight-3)
	}
	return
}

func (m *model) renderTranscript() string {
	m.transcriptToggles = map[int]string{}
	if len(m.transcript) == 0 {
		return m.styles.muted.Render("No messages yet. Type a prompt and press Enter.")
	}
	contentWidth := max(20, m.transcriptVP.Width-2)
	units := m.renderTranscriptUnits(contentWidth)
	parts := make([]string, 0, len(units))
	lineNo := 1
	for i, unit := range units {
		parts = append(parts, unit.text)
		if unit.toggleID != "" {
			m.transcriptToggles[lineNo] = unit.toggleID
		}
		lineNo += lineCount(unit.text)
		if i < len(units)-1 {
			lineNo += 2
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func (m *model) renderSidebar() string {
	var blocks []string
	blocks = append(blocks, m.renderRuntimeBlock())
	blocks = append(blocks, m.renderSessionsBlock())
	return strings.Join(blocks, "\n\n")
}

func (m *model) renderInfoPanel(outerWidth, outerHeight int) string {
	return m.renderViewportWithIndicator("Info", &m.infoVP, outerWidth, outerHeight)
}

func (m *model) renderRuntimeBlock() string {
	lines := []string{
		m.styles.section.Render("Runtime"),
		fmt.Sprintf("cwd: %s", m.cwd),
	}
	if m.errorText != "" {
		lines = append(lines, m.styles.error.Render(m.errorText))
	}
	if !m.runtimeReady {
		lines = append(lines, m.styles.warning.Render("Copilot runtime is unavailable. Install `copilot` to enable live sessions."))
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderSessionsBlock() string {
	lines := []string{m.styles.section.Render("Sessions")}
	if len(m.sessions) == 0 {
		lines = append(lines, m.styles.muted.Render("No saved sessions loaded."))
		return strings.Join(lines, "\n")
	}
	limit := min(5, len(m.sessions))
	for _, item := range m.sessions[:limit] {
		title := item.Title
		if title == "" {
			title = shortID(item.ID)
		}
		lines = append(lines, fmt.Sprintf("%s  %s", shortID(item.ID), title))
		lines = append(lines, m.styles.muted.Render(item.UpdatedAt.Format(time.RFC822)))
	}
	if len(m.sessions) > limit {
		lines = append(lines, m.styles.muted.Render("more..."))
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderActivityBlock() string {
	lines := []string{}
	if len(m.activity) == 0 {
		lines = append(lines, m.styles.muted.Render("Waiting for runtime events."))
		return strings.Join(lines, "\n")
	}
	for _, item := range m.activity {
		stamp := item.When.Format("15:04:05")
		line := fmt.Sprintf("%s [%s] %s", stamp, item.Kind, item.Text)
		if item.Status == "error" {
			line = m.styles.error.Render(line)
		} else if item.Status == "blocked" {
			line = m.styles.warning.Render(line)
		}
		lines = append(lines, line)
		if item.Detail != "" {
			lines = append(lines, m.styles.muted.Render(wrap(item.Detail, max(16, m.activityVP.Width-2))))
		}
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderTranscriptEntry(item state.TranscriptEntry, width int) string {
	label, labelStyle := m.transcriptLabel(item)
	meta := []string{}
	if !item.When.IsZero() {
		meta = append(meta, item.When.Format("15:04:05"))
	}
	if item.Status != "" && item.Status != "ready" {
		meta = append(meta, item.Status)
	}
	header := labelStyle.Render(strings.ToUpper(label))
	if len(meta) > 0 {
		header += " " + m.styles.muted.Render(strings.Join(meta, "  "))
	}
	body := m.transcriptBody(item)
	parts := []string{header}
	if body != "" {
		parts = append(parts, wrap(body, width))
	}
	if item.Detail != "" {
		parts = append(parts, m.styles.muted.Render(wrap(item.Detail, width)))
	}
	return strings.Join(parts, "\n")
}

type transcriptUnit struct {
	text     string
	toggleID string
}

func (m *model) renderTranscriptUnits(width int) []transcriptUnit {
	units := make([]transcriptUnit, 0, len(m.transcript))
	for i := 0; i < len(m.transcript); {
		item := m.transcript[i]
		groupID := m.executionGroupID(item)
		if groupID == "" {
			units = append(units, transcriptUnit{text: m.renderTranscriptEntry(item, width)})
			i++
			continue
		}
		j := i + 1
		for j < len(m.transcript) && m.executionGroupID(m.transcript[j]) == groupID {
			j++
		}
		units = append(units, transcriptUnit{text: m.renderExecutionBlock(groupID, m.transcript[i:j], width), toggleID: groupID})
		i = j
	}
	return units
}

func (m *model) renderExecutionBlock(groupID string, items []state.TranscriptEntry, width int) string {
	marker := "[-]"
	if m.executionCollapsed(groupID) {
		marker = "[+]"
	}
	label, labelStyle := m.executionBlockLabel(items)
	summary := truncate(m.executionBlockSummary(items), max(12, width/2))
	meta := []string{}
	if !items[0].When.IsZero() {
		meta = append(meta, items[0].When.Format("15:04:05"))
	}
	status := m.executionBlockStatus(items)
	if status != "" && status != "ready" {
		meta = append(meta, status)
	}
	meta = append(meta, fmt.Sprintf("%d events", len(items)))
	header := fmt.Sprintf("%s %s %s", marker, labelStyle.Render(strings.ToUpper(label)), summary)
	if len(meta) > 0 {
		header += "  " + m.styles.muted.Render(strings.Join(meta, "  "))
	}
	if m.executionCollapsed(groupID) {
		return header
	}
	parts := []string{header}
	childWidth := max(16, width-2)
	for _, item := range items {
		parts = append(parts, indentBlock(m.renderTranscriptEntry(item, childWidth), "| "))
	}
	return strings.Join(parts, "\n")
}

func (m *model) executionGroupID(item state.TranscriptEntry) string {
	if item.ID == "" {
		return ""
	}
	switch item.Kind {
	case "tool", "tool_progress", "tool_result", "permission", "subagent", "subagent_result", "subagent_failed":
		if item.Kind == "subagent" && strings.HasPrefix(item.Text, "selected ") {
			return ""
		}
		return item.ID
	default:
		return ""
	}
}

func (m *model) executionBlockLabel(items []state.TranscriptEntry) (string, lipgloss.Style) {
	for _, item := range items {
		switch item.Kind {
		case "subagent", "subagent_result", "subagent_failed":
			if item.Status == "error" {
				return "subagent", m.styles.error
			}
			return "subagent", m.styles.subagent
		case "tool", "tool_progress", "tool_result", "permission":
			if item.Status == "error" {
				return "tool", m.styles.error
			}
			return "tool", m.styles.tool
		}
	}
	return "execution", m.styles.section
}

func (m *model) executionBlockSummary(items []state.TranscriptEntry) string {
	for _, item := range items {
		if item.Kind == "tool" && item.Text != "" {
			return item.Text
		}
	}
	for _, item := range items {
		if strings.HasPrefix(item.Kind, "subagent") && item.Text != "" {
			return item.Text
		}
	}
	for _, item := range items {
		if item.Text != "" {
			return item.Text
		}
	}
	return "execution"
}

func (m *model) executionBlockStatus(items []state.TranscriptEntry) string {
	status := ""
	for _, item := range items {
		switch item.Status {
		case "error":
			return "error"
		case "blocked":
			status = "blocked"
		case "running":
			if status == "" || status == "ready" {
				status = "running"
			}
		case "ready":
			if status == "" {
				status = "ready"
			}
		}
	}
	return status
}

func (m *model) transcriptBody(item state.TranscriptEntry) string {
	switch item.Kind {
	case "tool_progress":
		if name := m.toolCalls[item.ID]; name != "" {
			return fmt.Sprintf("%s: %s", name, item.Text)
		}
	case "tool_result":
		if name := m.toolCalls[item.ID]; name != "" {
			return fmt.Sprintf("%s %s", name, item.Text)
		}
	}
	return item.Text
}

func (m *model) transcriptLabel(item state.TranscriptEntry) (string, lipgloss.Style) {
	switch {
	case item.Role == "user":
		return "you", m.styles.user
	case item.Role == "assistant":
		return "assistant", m.styles.assistant
	}
	switch item.Kind {
	case "reasoning":
		return "reasoning", m.styles.reasoning
	case "intent":
		return "intent", m.styles.reasoning
	case "tool", "tool_progress", "tool_result":
		return "tool", m.styles.tool
	case "permission", "permission_result":
		if item.Status == "blocked" {
			return "permission", m.styles.warning
		}
		return "permission", m.styles.permission
	case "subagent", "subagent_result", "subagent_failed":
		if item.Status == "error" {
			return "subagent", m.styles.error
		}
		return "subagent", m.styles.subagent
	default:
		if item.Status == "error" {
			return item.Kind, m.styles.error
		}
		return item.Kind, m.styles.section
	}
}

func (m *model) renderFooter() string {
	inputFrameW, _ := m.styles.input.GetFrameSize()
	input := m.styles.input.Width(max(1, m.width-inputFrameW)).Render(m.input.View())
	hint := "Enter send  click execution headers collapse  arrows transcript  Alt+Arrows info  Shift+Arrows activity  /quit exit"
	if m.commandHint != "" {
		hint = m.commandHint
	}
	shortcuts := m.styles.muted.Render(truncate(hint, max(1, m.width)))
	return lipgloss.JoinVertical(lipgloss.Left, input, shortcuts)
}

func (m *model) renderTranscriptPanel(outerWidth, outerHeight int) string {
	frameW, frameH := m.styles.panel.GetFrameSize()
	innerWidth := max(1, outerWidth-frameW)
	innerHeight := max(1, outerHeight-frameH)
	bodyHeight := max(1, innerHeight-2)
	indicator := m.scrollIndicator(&m.transcriptVP)
	left := m.styles.section.Render("Transcript")
	indicatorText := truncate(indicator, max(1, innerWidth-lipgloss.Width(left)-1))
	gapWidth := max(1, innerWidth-lipgloss.Width(left)-lipgloss.Width(indicatorText))
	headerLine := left + strings.Repeat(" ", gapWidth) + m.styles.muted.Render(indicatorText)
	header := lipgloss.NewStyle().Width(innerWidth).MaxWidth(innerWidth).Render(headerLine)
	body := lipgloss.NewStyle().Width(innerWidth).Height(bodyHeight).Render(m.transcriptVP.View())
	status := m.styles.transcriptStatus.Width(innerWidth).MaxWidth(innerWidth).Render(truncate(m.renderTranscriptStatusLine(), innerWidth))
	return m.styles.panel.Render(lipgloss.JoinVertical(lipgloss.Left, header, body, status))
}

func (m *model) renderTranscriptStatusLine() string {
	frame := sisyphusFrames[m.animationFrame%len(sisyphusFrames)]
	if !m.isWorking() {
		return frame + "  idle  click execution headers to expand or collapse details"
	}
	status := m.status
	if status == "" {
		status = "working"
	}
	return frame + "  " + status + "  sysiphus is still pushing"
}

func (m *model) renderViewportWithIndicator(title string, vp *viewport.Model, outerWidth, outerHeight int) string {
	frameW, frameH := m.styles.panel.GetFrameSize()
	innerWidth := max(1, outerWidth-frameW)
	innerHeight := max(1, outerHeight-frameH)
	bodyHeight := max(1, innerHeight-1)
	indicator := m.scrollIndicator(vp)
	body := lipgloss.NewStyle().Width(innerWidth).Height(bodyHeight).Render(vp.View())
	if title == "" {
		header := lipgloss.NewStyle().Width(innerWidth).MaxWidth(innerWidth).Render(truncate(indicator, max(1, innerWidth)))
		return m.styles.panel.Render(lipgloss.JoinVertical(lipgloss.Left, header, body))
	}
	left := m.styles.section.Render(title)
	available := innerWidth
	indicatorText := truncate(indicator, max(1, available-lipgloss.Width(title)-1))
	gapWidth := max(1, available-lipgloss.Width(title)-lipgloss.Width(indicatorText))
	headerLine := left + strings.Repeat(" ", gapWidth) + m.styles.muted.Render(indicatorText)
	header := lipgloss.NewStyle().Width(available).MaxWidth(available).Render(headerLine)
	return m.styles.panel.Render(lipgloss.JoinVertical(lipgloss.Left, header, body))
}

func (m *model) scrollIndicator(vp *viewport.Model) string {
	if vp.TotalLineCount() <= vp.Height {
		return ""
	}
	top := " "
	bottom := " "
	if vp.YOffset > 0 {
		top = "^"
	}
	if vp.YOffset+vp.Height < vp.TotalLineCount() {
		bottom = "v"
	}
	return fmt.Sprintf("%s %d/%d %s", top, vp.YOffset+1, vp.TotalLineCount(), bottom)
}

func (m *model) updatePaneRects() {
	if m.width == 0 || m.height == 0 {
		return
	}
	bodyHeight, leftWidth, rightWidth, rightTopHeight, _ := m.layoutMetrics()
	if bodyHeight == 0 {
		m.transcriptRect = rect{}
		m.infoRect = rect{}
		m.activityRect = rect{}
		return
	}
	headerHeight := lipgloss.Height(m.renderHeader())
	panelFrameW, panelFrameH := m.styles.panel.GetFrameSize()
	const colGap = 1
	m.transcriptRect = rect{
		x:      1,
		y:      headerHeight + 2,
		width:  max(1, leftWidth-panelFrameW),
		height: max(1, bodyHeight-panelFrameH-2),
	}
	rightX := leftWidth + colGap + 1
	m.infoRect = rect{
		x:      rightX,
		y:      headerHeight + 2,
		width:  max(1, rightWidth-panelFrameW),
		height: max(1, rightTopHeight-panelFrameH-1),
	}
	m.activityRect = rect{
		x:      rightX,
		y:      headerHeight + rightTopHeight + 2,
		width:  max(1, rightWidth-panelFrameW),
		height: max(1, bodyHeight-rightTopHeight-panelFrameH-1),
	}
}

func (m *model) renderModelPicker(base string) string {
	content := m.modelPicker.View()
	box := m.styles.modal.Render(content)
	overlayX := max(0, (m.width-lipgloss.Width(box))/2)
	overlayY := max(0, (m.height-lipgloss.Height(box))/2)
	overlay := lipgloss.NewStyle().MarginLeft(overlayX).MarginTop(overlayY).Render(box)
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, base) +
		lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, overlay)
}

func (m *model) renderSessionPicker(base string) string {
	content := m.sessionPicker.View()
	box := m.styles.modal.Render(content)
	overlayX := max(0, (m.width-lipgloss.Width(box))/2)
	overlayY := max(0, (m.height-lipgloss.Height(box))/2)
	overlay := lipgloss.NewStyle().MarginLeft(overlayX).MarginTop(overlayY).Render(box)
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, base) +
		lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, overlay)
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func wrap(text string, width int) string {
	if width < 10 {
		return text
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped = append(wrapped, wrapLine(line, width)...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapLine(line string, width int) []string {
	if strings.TrimSpace(line) == "" {
		return []string{""}
	}
	if len([]rune(line)) <= width || shouldPreserveLine(line) {
		return []string{line}
	}
	indent := leadingWhitespace(line)
	content := strings.TrimLeft(line, " \t")
	words := strings.Fields(content)
	if len(words) == 0 {
		return []string{line}
	}
	lines := []string{}
	current := indent + words[0]
	for _, word := range words[1:] {
		if len([]rune(current))+1+len([]rune(word)) > width {
			lines = append(lines, current)
			current = indent + word
			continue
		}
		current += " " + word
	}
	lines = append(lines, current)
	return lines
}

func shouldPreserveLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(trimmed, "```")
}

func leadingWhitespace(line string) string {
	end := 0
	for end < len(line) {
		if line[end] != ' ' && line[end] != '\t' {
			break
		}
		end++
	}
	return line[:end]
}

func truncate(text string, maxLen int) string {
	if len([]rune(text)) <= maxLen {
		return text
	}
	return string([]rune(text)[:maxLen-1]) + "…"
}

func indentBlock(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func lineCount(text string) int {
	if text == "" {
		return 1
	}
	return strings.Count(text, "\n") + 1
}

var sisyphusFrames = []string{
	" _o/  O",
	" _o/ O ",
	" _o/O  ",
	" _o\\O  ",
	" _o_\\O ",
	" _o__\\O",
}
