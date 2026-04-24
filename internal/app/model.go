package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matt/sysiphus/internal/agents"
	copilotbridge "github.com/matt/sysiphus/internal/copilot"
	"github.com/matt/sysiphus/internal/sessionstore"
	"github.com/matt/sysiphus/internal/state"
)

type model struct {
	ctx               context.Context
	cancel            context.CancelFunc
	runtime           *copilotbridge.Runtime
	input             textinput.Model
	transcriptVP      viewport.Model
	infoVP            viewport.Model
	activityVP        viewport.Model
	width             int
	height            int
	transcript        []state.TranscriptEntry
	activity          []state.ActivityEntry
	sessions          []state.SessionSummary
	availableModels   []state.ModelOption
	store             *sessionstore.Store
	localSessionID    string
	status            string
	errorText         string
	cwd               string
	branch            string
	mode              string
	policy            state.ApprovalPolicy
	modelName         string
	sessionID         string
	booted            bool
	showHelp          bool
	runtimeReady      bool
	pendingPrompt     bool
	commandHint       string
	modelPicker       list.Model
	showModelPicker   bool
	sessionPicker     list.Model
	showSessionPicker bool
	transcriptRect    rect
	infoRect          rect
	activityRect      rect
	toolCalls         map[string]string
	collapsedExecs    map[string]bool
	transcriptToggles map[int]string
	animationFrame    int
	styles            styles
}

type rect struct {
	x      int
	y      int
	width  int
	height int
}

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.width && y >= r.y && y < r.y+r.height
}

type runtimeEventMsg struct{ event copilotbridge.AppEvent }
type runtimeStartedMsg struct{ err error }
type animationTickMsg struct{}
type sessionStartedMsg struct {
	id                string
	resumed           bool
	providerSessionID string
	err               error
}
type sessionsLoadedMsg struct {
	items []state.SessionSummary
	err   error
}
type promptSentMsg struct{ err error }

func New() tea.Model {
	input := textinput.New()
	input.Placeholder = "Ask sysiphus to inspect, plan, or code"
	input.Focus()
	input.CharLimit = 0
	input.Width = 80

	ctx, cancel := context.WithCancel(context.Background())
	cwd, _ := os.Getwd()

	return &model{
		ctx:               ctx,
		cancel:            cancel,
		runtime:           copilotbridge.NewRuntime(),
		store:             sessionstore.New(cwd),
		input:             input,
		transcriptVP:      viewport.New(0, 0),
		infoVP:            viewport.New(0, 0),
		activityVP:        viewport.New(0, 0),
		status:            "booting",
		cwd:               cwd,
		mode:              agents.ModeBuild,
		policy:            state.ApprovalAllowAll,
		modelName:         "gpt-5",
		toolCalls:         map[string]string{},
		collapsedExecs:    map[string]bool{},
		transcriptToggles: map[int]string{},
		styles:            defaultStyles(),
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.startRuntimeCmd(), m.waitForRuntimeEvent(), m.animationTickCmd())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if !m.booted {
		m.booted = true
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = max(20, msg.Width-4)
		m.resizeViewports()
		m.refreshPaneContent(false)
	case runtimeStartedMsg:
		if msg.err != nil {
			m.runtimeReady = false
			m.status = "runtime unavailable"
			m.errorText = msg.err.Error()
			m.refreshPaneContent(false)
			return m, nil
		}
		m.runtimeReady = true
		m.status = "runtime ready"
		m.refreshPaneContent(false)
		return m, tea.Batch(m.newSessionCmd(), m.loadSessionsCmd(), m.loadModelsCmd(), m.waitForRuntimeEvent())
	case sessionStartedMsg:
		if msg.err != nil {
			m.pendingPrompt = false
			m.errorText = msg.err.Error()
			m.status = "session error"
			m.refreshPaneContent(false)
			return m, nil
		}
		m.localSessionID = msg.id
		m.sessionID = msg.providerSessionID
		m.pendingPrompt = false
		m.status = "session ready"
		_ = m.store.UpdateSession(m.localSessionID, func(meta *sessionstore.SessionMeta) {
			meta.ProviderSessionID = msg.providerSessionID
			meta.Model = m.modelName
			meta.Mode = m.mode
			meta.ApprovalPolicy = m.policy
		})
		m.refreshPaneContent(false)
		return m, m.loadSessionsCmd()
	case sessionsLoadedMsg:
		if msg.err == nil {
			m.sessions = msg.items
			m.refreshSessionPicker()
			m.refreshPaneContent(false)
		}
	case modelsLoadedMsg:
		if msg.err != nil {
			m.errorText = msg.err.Error()
			m.appendActivity("model", "failed to load models", msg.err.Error(), "error")
			m.refreshPaneContent(true)
			return m, nil
		}
		m.availableModels = msg.items
		m.refreshModelPicker()
		m.refreshPaneContent(false)
	case sessionLoadedMsg:
		if msg.err != nil {
			m.errorText = msg.err.Error()
			m.appendActivity("sessions", "failed to load session", msg.err.Error(), "error")
			m.refreshPaneContent(true)
			return m, nil
		}
		m.restoreStoredSession(msg.meta, msg.events)
		m.refreshPaneContent(false)
		if msg.meta.ProviderSessionID != "" {
			m.appendActivity("sessions", fmt.Sprintf("loaded local session %s", msg.meta.LocalID), "attempting provider resume next time /resume is invoked", "ready")
			m.refreshPaneContent(true)
		}
	case modelSwitchedMsg:
		if msg.err != nil {
			m.errorText = msg.err.Error()
			m.appendActivity("model", "failed to switch model", msg.err.Error(), "error")
			m.refreshPaneContent(true)
			return m, nil
		}
		m.modelName = msg.modelID
		m.appendActivity("model", fmt.Sprintf("model switched to %s", msg.modelID), "", "ready")
		m.refreshPaneContent(true)
	case runtimeEventMsg:
		m.consumeRuntimeEvent(msg.event)
		return m, m.waitForRuntimeEvent()
	case promptSentMsg:
		if msg.err != nil {
			m.pendingPrompt = false
			m.errorText = msg.err.Error()
			m.status = "send failed"
		}
		m.refreshPaneContent(true)
	case animationTickMsg:
		if m.isWorking() {
			m.animationFrame = (m.animationFrame + 1) % len(sisyphusFrames)
			m.refreshPaneContent(false)
		}
		return m, m.animationTickCmd()
	case tea.KeyMsg:
		if m.showModelPicker {
			return m.updateModelPicker(msg)
		}
		if m.showSessionPicker {
			return m.updateSessionPicker(msg)
		}
		switch msg.String() {
		case "ctrl+c":
			m.cancel()
			_ = m.runtime.Stop()
			return m, tea.Quit
		case "f1", "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "ctrl+up":
			m.transcriptVP.LineUp(1)
			return m, nil
		case "ctrl+down":
			m.transcriptVP.LineDown(1)
			return m, nil
		case "up":
			m.transcriptVP.LineUp(1)
			return m, nil
		case "down":
			m.transcriptVP.LineDown(1)
			return m, nil
		case "pgup":
			m.transcriptVP.HalfViewUp()
			return m, nil
		case "pgdown":
			m.transcriptVP.HalfViewDown()
			return m, nil
		case "alt+up":
			m.infoVP.LineUp(1)
			return m, nil
		case "alt+down":
			m.infoVP.LineDown(1)
			return m, nil
		case "alt+pgup":
			m.infoVP.HalfViewUp()
			return m, nil
		case "alt+pgdown":
			m.infoVP.HalfViewDown()
			return m, nil
		case "shift+up":
			m.activityVP.LineUp(1)
			return m, nil
		case "shift+down":
			m.activityVP.LineDown(1)
			return m, nil
		case "shift+pgup":
			m.activityVP.HalfViewUp()
			return m, nil
		case "shift+pgdown":
			m.activityVP.HalfViewDown()
			return m, nil
		case "tab":
			if !m.runtimeReady || m.pendingPrompt {
				return m, nil
			}
			m.setModeDefaults(nextMode(m.mode))
			m.appendActivity("mode", fmt.Sprintf("switched to %s agent", m.mode), fmt.Sprintf("starting a new session with %s approvals", m.policy), "running")
			m.refreshPaneContent(true)
			return m, tea.Batch(m.newSessionCmd(), m.loadSessionsCmd())
		case "ctrl+n":
			if m.runtimeReady {
				return m, m.newSessionCmd()
			}
		case "ctrl+r":
			if m.runtimeReady {
				return m, tea.Batch(m.resumeLatestCmd(), m.loadSessionsCmd())
			}
		case "ctrl+l":
			m.clearLocalPanes()
			m.refreshPaneContent(false)
			return m, nil
		case "ctrl+a":
			if m.policy == state.ApprovalConservative {
				m.policy = state.ApprovalAllowAll
			} else {
				m.policy = state.ApprovalConservative
			}
			m.runtime.SetApprovalPolicy(m.policy)
			detail := ""
			if m.runtimeReady && m.sessionID != "" {
				detail = "reconnecting active session so the new policy takes effect"
				m.appendActivity("policy", fmt.Sprintf("approval policy set to %s", m.policy), detail, "running")
				m.refreshPaneContent(true)
				return m, m.refreshActiveSessionCmd()
			}
			m.appendActivity("policy", fmt.Sprintf("approval policy set to %s", m.policy), detail, "ready")
			m.refreshPaneContent(true)
			return m, nil
		case "ctrl+p":
			m.setModeDefaults(nextMode(m.mode))
			detail := fmt.Sprintf("new sessions use %s approvals", m.policy)
			if m.runtimeReady && m.sessionID != "" {
				detail = fmt.Sprintf("reconnecting active session with %s approvals", m.policy)
				m.appendActivity("mode", fmt.Sprintf("agent mode set to %s", m.mode), detail, "running")
				m.refreshPaneContent(true)
				return m, m.refreshActiveSessionCmd()
			}
			m.appendActivity("mode", fmt.Sprintf("agent mode set to %s", m.mode), detail, "ready")
			m.refreshPaneContent(true)
			return m, nil
		case "enter":
			prompt := strings.TrimSpace(m.input.Value())
			if prompt == "" {
				return m, nil
			}
			if strings.HasPrefix(prompt, "/") {
				return m, m.runSlashCommandCmd(prompt)
			}
			if !m.runtimeReady || m.sessionID == "" || m.pendingPrompt {
				return m, nil
			}
			m.pendingPrompt = true
			m.status = "sending"
			m.appendTranscriptEntry("user", "user", "", prompt, "", "ready")
			m.recordEvent("user", prompt, "", "ready", "")
			m.input.SetValue("")
			m.commandHint = ""
			m.refreshPaneContent(true)
			return m, m.sendPromptCmd(prompt)
		}
	}

	if mouseMsg, ok := msg.(tea.MouseMsg); ok {
		if mouseMsg.Button == tea.MouseButtonWheelUp || mouseMsg.Button == tea.MouseButtonWheelDown {
			m.scrollViewportAt(mouseMsg.X, mouseMsg.Y, mouseMsg.Button == tea.MouseButtonWheelUp)
			return m, nil
		}
		if mouseMsg.Button == tea.MouseButtonLeft && m.toggleTranscriptBlockAt(mouseMsg.X, mouseMsg.Y) {
			m.refreshPaneContent(false)
			return m, nil
		}
	}

	m.input, cmd = m.input.Update(msg)
	m.commandHint = slashCommandHint(m.input.Value())
	m.refreshPaneContent(false)
	return m, cmd
}

func (m *model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading sysiphus..."
	}
	m.updatePaneRects()
	header := m.renderHeader()
	bodyHeight, leftWidth, rightWidth, rightTopHeight, rightBottomHeight := m.layoutMetrics()

	left := m.renderTranscriptPanel(leftWidth, bodyHeight)
	rightTop := m.renderInfoPanel(rightWidth, rightTopHeight)
	rightBottom := m.renderViewportWithIndicator("Activity", &m.activityVP, rightWidth, rightBottomHeight)
	right := lipgloss.JoinVertical(lipgloss.Left, rightTop, rightBottom)
	row := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	footer := m.renderFooter()
	view := lipgloss.JoinVertical(lipgloss.Left, header, row, footer)
	if m.showHelp {
		view = lipgloss.JoinVertical(lipgloss.Left, view, m.styles.help.Render(helpText()))
	}
	if m.showModelPicker {
		view = m.renderModelPicker(view)
	}
	if m.showSessionPicker {
		view = m.renderSessionPicker(view)
	}
	return view
}

func (m *model) startRuntimeCmd() tea.Cmd {
	return func() tea.Msg {
		return runtimeStartedMsg{err: m.runtime.Start(m.ctx, m.cwd)}
	}
}

func (m *model) newSessionCmd() tea.Cmd {
	mode := m.mode
	modelName := m.modelName
	policy := m.policy
	return func() tea.Msg {
		meta, err := m.store.CreateSession(sessionstore.SessionMeta{
			Provider:       "copilot",
			Model:          modelName,
			Mode:           mode,
			ApprovalPolicy: policy,
		})
		if err != nil {
			return sessionStartedMsg{err: err}
		}
		m.runtime.SetMode(mode)
		m.runtime.SetApprovalPolicy(policy)
		providerID, err := m.runtime.NewSession(m.ctx, m.cwd, modelName)
		return sessionStartedMsg{id: meta.LocalID, providerSessionID: providerID, err: err}
	}
}

func (m *model) resumeLatestCmd() tea.Cmd {
	return func() tea.Msg {
		latest, err := m.store.LatestSession()
		if err != nil {
			return sessionStartedMsg{err: err}
		}
		if latest == nil {
			return sessionStartedMsg{err: fmt.Errorf("no previous local session found")}
		}
		if latest.ProviderSessionID == "" {
			return sessionStartedMsg{err: fmt.Errorf("latest saved session has no provider session id")}
		}
		providerID, err := m.runtime.ResumeSession(m.ctx, latest.ProviderSessionID)
		return sessionStartedMsg{id: latest.ID, resumed: true, providerSessionID: providerID, err: err}
	}
}

func (m *model) loadSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := m.store.ListSessions()
		return sessionsLoadedMsg{items: items, err: err}
	}
}

func (m *model) loadSessionCmd(localID string) tea.Cmd {
	return func() tea.Msg {
		meta, events, err := m.store.LoadSession(localID)
		return sessionLoadedMsg{meta: meta, events: events, err: err}
	}
}

func (m *model) loadModelsCmd() tea.Cmd {
	return func() tea.Msg {
		items, err := m.runtime.ListModels(m.ctx)
		return modelsLoadedMsg{items: items, err: err}
	}
}

func (m *model) sendPromptCmd(prompt string) tea.Cmd {
	return func() tea.Msg {
		return promptSentMsg{err: m.runtime.Send(m.ctx, prompt)}
	}
}

func (m *model) refreshActiveSessionCmd() tea.Cmd {
	localID := m.localSessionID
	providerID := m.sessionID
	mode := m.mode
	policy := m.policy
	return func() tea.Msg {
		if providerID == "" {
			return sessionStartedMsg{err: fmt.Errorf("no active provider session to refresh")}
		}
		m.runtime.SetMode(mode)
		m.runtime.SetApprovalPolicy(policy)
		resumedID, err := m.runtime.ResumeSession(m.ctx, providerID)
		return sessionStartedMsg{id: localID, resumed: true, providerSessionID: resumedID, err: err}
	}
}

func (m *model) switchModelCmd(modelID string) tea.Cmd {
	return func() tea.Msg {
		return modelSwitchedMsg{modelID: modelID, err: m.runtime.SwitchModel(m.ctx, modelID)}
	}
}

func (m *model) runSlashCommandCmd(raw string) tea.Cmd {
	command := strings.TrimSpace(raw)
	m.input.SetValue("")
	m.commandHint = ""
	parts := strings.Fields(strings.TrimPrefix(command, "/"))
	if len(parts) == 0 {
		m.appendActivity("command", "empty command", "", "error")
		m.refreshPaneContent(true)
		return nil
	}
	name := strings.ToLower(parts[0])
	args := parts[1:]
	switch name {
	case "new":
		m.appendActivity("command", "/new", "starting a fresh session", "ready")
		return tea.Batch(m.newSessionCmd(), m.loadSessionsCmd())
	case "resume":
		m.appendActivity("command", "/resume", "resuming the latest saved session", "ready")
		return tea.Batch(m.resumeLatestCmd(), m.loadSessionsCmd())
	case "sessions":
		m.showSessionPicker = true
		m.refreshSessionPicker()
		m.appendActivity("command", "/sessions", "opening workspace session picker", "ready")
		m.refreshPaneContent(true)
		return m.loadSessionsCmd()
	case "model":
		m.showModelPicker = true
		m.refreshModelPicker()
		m.appendActivity("command", "/model", "opening model selector", "ready")
		m.refreshPaneContent(true)
		return m.loadModelsCmd()
	case "plan":
		m.setModeDefaults(agents.ModePlan)
		m.appendActivity("command", "/plan", "switching to the plan agent and starting a fresh session", "ready")
		return tea.Batch(m.newSessionCmd(), m.loadSessionsCmd())
	case "build":
		m.setModeDefaults(agents.ModeBuild)
		m.appendActivity("command", "/build", "switching to the build agent and starting a fresh session", "ready")
		return tea.Batch(m.newSessionCmd(), m.loadSessionsCmd())
	case "mode":
		if len(args) == 0 {
			m.appendActivity("command", "/mode", fmt.Sprintf("current mode is %s", m.mode), "ready")
			m.refreshPaneContent(true)
			return nil
		}
		wanted := strings.ToLower(args[0])
		if wanted != agents.ModeBuild && wanted != agents.ModePlan {
			m.appendActivity("command", "/mode", "usage: /mode build|plan", "error")
			m.refreshPaneContent(true)
			return nil
		}
		m.setModeDefaults(wanted)
		m.appendActivity("command", "/mode", fmt.Sprintf("switched to %s and started a fresh session", m.mode), "ready")
		return tea.Batch(m.newSessionCmd(), m.loadSessionsCmd())
	case "approvals":
		if len(args) == 0 {
			m.appendActivity("command", "/approvals", fmt.Sprintf("current approval policy is %s", m.policy), "ready")
			m.refreshPaneContent(true)
			return nil
		}
		wanted := strings.ToLower(args[0])
		switch wanted {
		case string(state.ApprovalConservative):
			m.policy = state.ApprovalConservative
		case string(state.ApprovalAllowAll):
			m.policy = state.ApprovalAllowAll
		default:
			m.appendActivity("command", "/approvals", "usage: /approvals conservative|allow-all", "error")
			m.refreshPaneContent(true)
			return nil
		}
		m.runtime.SetApprovalPolicy(m.policy)
		detail := ""
		if m.runtimeReady && m.sessionID != "" {
			detail = "reconnecting active session so the new policy takes effect"
			m.appendActivity("command", "/approvals", fmt.Sprintf("approval policy set to %s", m.policy), "running")
			m.refreshPaneContent(true)
			return m.refreshActiveSessionCmd()
		}
		m.appendActivity("command", fmt.Sprintf("approval policy set to %s", m.policy), detail, "ready")
		m.refreshPaneContent(true)
		return nil
	case "clear":
		m.clearLocalPanes()
		m.appendActivity("command", "/clear", "cleared local transcript and activity panes", "ready")
		m.refreshPaneContent(false)
		return nil
	case "help":
		m.showHelp = true
		m.appendActivity("command", "/help", "showing command help", "ready")
		m.refreshPaneContent(true)
		return nil
	case "quit", "exit":
		m.cancel()
		_ = m.runtime.Stop()
		return tea.Quit
	default:
		m.appendActivity("command", command, "unknown command", "error")
		m.refreshPaneContent(true)
		return nil
	}
}

func (m *model) waitForRuntimeEvent() tea.Cmd {
	return func() tea.Msg {
		event, ok := <-m.runtime.Events()
		if !ok {
			return nil
		}
		return runtimeEventMsg{event: event}
	}
}

func (m *model) animationTickCmd() tea.Cmd {
	return tea.Tick(160*time.Millisecond, func(time.Time) tea.Msg {
		return animationTickMsg{}
	})
}

func (m *model) consumeRuntimeEvent(event copilotbridge.AppEvent) {
	m.applyEvent(time.Now(), event)
	if event.Session != "" {
		m.sessionID = event.Session
		if m.localSessionID != "" {
			_ = m.store.UpdateSession(m.localSessionID, func(meta *sessionstore.SessionMeta) {
				meta.ProviderSessionID = event.Session
			})
		}
	}
	m.recordEvent(event.Kind, event.Text, event.Detail, event.Status, event.Ref)
	m.refreshPaneContent(true)
}

func (m *model) applyEvent(when time.Time, event copilotbridge.AppEvent) {
	switch event.Kind {
	case "assistant_delta":
		m.appendAssistantDelta(event.Text)
		m.status = event.Status
	case "turn":
		m.pendingPrompt = event.Status == "running"
		m.status = event.Status
	case "assistant":
		m.replaceLatestAssistant(event.Text)
		m.pendingPrompt = false
		m.status = event.Status
	case "reasoning_delta":
		m.appendReasoningDelta(event.Ref, event.Text)
		m.status = event.Status
	case "reasoning":
		m.replaceLatestReasoning(event.Ref, event.Text)
		m.status = event.Status
	case "user":
		if len(m.transcript) == 0 || m.transcript[len(m.transcript)-1].Text != event.Text {
			m.appendTranscriptEntryAt(when, "user", "user", event.Ref, event.Text, "", event.Status)
		}
	case "intent", "tool", "tool_progress", "tool_result", "permission", "permission_result", "subagent", "subagent_result", "subagent_failed":
		if event.Kind == "tool" && event.Ref != "" {
			m.toolCalls[event.Ref] = event.Text
		}
		if event.Kind == "permission_result" {
			m.resolvePermissionBlock(event)
			m.appendActivity(event.Kind, event.Text, event.Detail, event.Status)
			m.status = event.Status
			break
		}
		m.appendTranscriptEntryAt(when, "", event.Kind, event.Ref, event.Text, event.Detail, event.Status)
		m.appendActivity(event.Kind, event.Text, event.Detail, event.Status)
		m.status = event.Status
	case "context":
		m.cwd = event.Text
		m.branch = event.Detail
	case "status", "idle", "usage", "activity", "error":
		if event.Kind == "idle" || event.Kind == "error" {
			m.pendingPrompt = false
		}
		if event.Kind == "error" {
			m.errorText = event.Text
		}
		m.appendActivity(event.Kind, event.Text, event.Detail, event.Status)
		m.status = event.Status
	case "model":
		m.modelName = event.Text
		detail := event.Detail
		if detail == "" {
			detail = "session model updated"
		}
		m.appendActivity("model", fmt.Sprintf("active model %s", event.Text), detail, event.Status)
		m.status = event.Status
	}
}

func (m *model) appendAssistantDelta(delta string) {
	if delta == "" {
		return
	}
	if len(m.transcript) == 0 || m.transcript[len(m.transcript)-1].Role != "assistant" {
		m.appendTranscriptEntry("assistant", "assistant", "", delta, "", "running")
		return
	}
	m.transcript[len(m.transcript)-1].Text += delta
	m.transcript[len(m.transcript)-1].Status = "running"
}

func (m *model) replaceLatestAssistant(text string) {
	if text == "" {
		return
	}
	if len(m.transcript) == 0 || m.transcript[len(m.transcript)-1].Role != "assistant" {
		m.appendTranscriptEntry("assistant", "assistant", "", text, "", "ready")
		return
	}
	m.transcript[len(m.transcript)-1].Text = text
	m.transcript[len(m.transcript)-1].Status = "ready"
}

func (m *model) appendReasoningDelta(id, delta string) {
	if delta == "" {
		return
	}
	idx := m.findTranscriptEntry("reasoning", id)
	if idx == -1 {
		m.appendTranscriptEntry("", "reasoning", id, delta, "", "running")
		return
	}
	m.transcript[idx].Text += delta
	m.transcript[idx].Status = "running"
}

func (m *model) replaceLatestReasoning(id, text string) {
	if text == "" {
		return
	}
	idx := m.findTranscriptEntry("reasoning", id)
	if idx == -1 {
		m.appendTranscriptEntry("", "reasoning", id, text, "", "running")
		return
	}
	m.transcript[idx].Text = text
	m.transcript[idx].Status = "running"
}

func (m *model) appendActivity(kind, text, detail, status string) {
	m.activity = append(m.activity, state.ActivityEntry{
		When:   time.Now(),
		Kind:   kind,
		Text:   text,
		Detail: detail,
		Status: status,
	})
	if len(m.activity) > 200 {
		m.activity = m.activity[len(m.activity)-200:]
	}
}

func (m *model) resizeViewports() {
	if m.width == 0 || m.height == 0 {
		return
	}
	bodyHeight, leftWidth, rightWidth, rightTopHeight, rightBottomHeight := m.layoutMetrics()
	panelFrameW, panelFrameH := m.styles.panel.GetFrameSize()
	m.transcriptVP.Width = max(1, leftWidth-panelFrameW)
	m.transcriptVP.Height = max(1, bodyHeight-panelFrameH-2)
	m.infoVP.Width = max(1, rightWidth-panelFrameW)
	m.infoVP.Height = max(1, rightTopHeight-panelFrameH-1)
	m.activityVP.Width = max(1, rightWidth-panelFrameW)
	m.activityVP.Height = max(1, rightBottomHeight-panelFrameH-1)
}

func (m *model) refreshPaneContent(followBottom bool) {
	m.transcriptVP.SetContent(m.renderTranscript())
	m.infoVP.SetContent(m.renderSidebar())
	m.activityVP.SetContent(m.renderActivityBlock())
	m.updatePaneRects()
	if followBottom {
		m.transcriptVP.GotoBottom()
		m.infoVP.GotoTop()
		m.activityVP.GotoBottom()
	}
}

func (m *model) appendTranscriptEntry(role, kind, id, text, detail, status string) {
	m.appendTranscriptEntryAt(time.Now(), role, kind, id, text, detail, status)
}

func (m *model) appendTranscriptEntryAt(when time.Time, role, kind, id, text, detail, status string) {
	m.transcript = append(m.transcript, state.TranscriptEntry{
		When:   when,
		Role:   role,
		Kind:   kind,
		ID:     id,
		Text:   text,
		Detail: detail,
		Status: status,
	})
	if len(m.transcript) > 400 {
		m.transcript = m.transcript[len(m.transcript)-400:]
	}
}

func (m *model) findTranscriptEntry(kind, id string) int {
	for i := len(m.transcript) - 1; i >= 0; i-- {
		entry := m.transcript[i]
		if entry.Kind != kind {
			continue
		}
		if id == "" || entry.ID == id {
			return i
		}
	}
	return -1
}

func (m *model) recordEvent(kind, text, detail, status, ref string) {
	if m.store == nil || m.localSessionID == "" {
		return
	}
	_ = m.store.AppendEvent(m.localSessionID, sessionstore.EventRecord{
		Timestamp: time.Now().UTC(),
		Kind:      kind,
		Ref:       ref,
		Text:      text,
		Detail:    detail,
		Status:    status,
		Session:   m.sessionID,
	})
	_ = m.store.UpdateSession(m.localSessionID, func(meta *sessionstore.SessionMeta) {
		meta.Model = m.modelName
		meta.Mode = m.mode
		meta.ApprovalPolicy = m.policy
	})
}

func (m *model) refreshSessionPicker() {
	width := min(84, max(44, m.width-8))
	height := min(18, max(8, m.height-8))
	m.sessionPicker = newSessionList(width, height, m.sessions)
	if m.localSessionID == "" {
		return
	}
	for i, item := range m.sessions {
		if item.ID == m.localSessionID {
			m.sessionPicker.Select(i)
			break
		}
	}
}

func (m *model) updateSessionPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.showSessionPicker = false
			return m, nil
		case "enter":
			selected := m.sessionPicker.SelectedItem()
			item, ok := selected.(sessionItem)
			if !ok {
				m.showSessionPicker = false
				return m, nil
			}
			m.showSessionPicker = false
			return m, m.loadSessionCmd(item.summary.ID)
		}
	}
	var cmd tea.Cmd
	m.sessionPicker, cmd = m.sessionPicker.Update(msg)
	return m, cmd
}

func (m *model) restoreStoredSession(meta sessionstore.SessionMeta, events []sessionstore.EventRecord) {
	m.localSessionID = meta.LocalID
	m.sessionID = meta.ProviderSessionID
	if meta.Model != "" {
		m.modelName = meta.Model
	}
	if meta.Mode != "" {
		m.mode = meta.Mode
		m.runtime.SetMode(meta.Mode)
	}
	if meta.ApprovalPolicy != "" {
		m.policy = meta.ApprovalPolicy
		m.runtime.SetApprovalPolicy(meta.ApprovalPolicy)
	} else {
		m.policy = policyForMode(m.mode)
		m.runtime.SetApprovalPolicy(m.policy)
	}
	m.transcript = nil
	m.activity = nil
	m.toolCalls = map[string]string{}
	m.collapsedExecs = map[string]bool{}
	m.transcriptToggles = map[int]string{}
	for _, event := range events {
		m.applyEvent(event.Timestamp, copilotbridge.AppEvent{
			Kind:    event.Kind,
			Ref:     event.Ref,
			Parent:  "",
			Text:    event.Text,
			Detail:  event.Detail,
			Status:  event.Status,
			Session: event.Session,
		})
		if event.Session != "" {
			m.sessionID = event.Session
		}
	}
	m.status = "session loaded"
	m.pendingPrompt = false
}

func (m *model) setModeDefaults(mode string) {
	m.mode = mode
	m.policy = policyForMode(mode)
	m.runtime.SetMode(mode)
	m.runtime.SetApprovalPolicy(m.policy)
}

func nextMode(current string) string {
	if current == agents.ModeBuild {
		return agents.ModePlan
	}
	return agents.ModeBuild
}

func policyForMode(mode string) state.ApprovalPolicy {
	if mode == agents.ModePlan {
		return state.ApprovalConservative
	}
	return state.ApprovalAllowAll
}

func (m *model) clearLocalPanes() {
	m.transcript = nil
	m.activity = nil
	m.toolCalls = map[string]string{}
	m.collapsedExecs = map[string]bool{}
	m.transcriptToggles = map[int]string{}
	m.errorText = ""
}

func (m *model) resolvePermissionBlock(event copilotbridge.AppEvent) {
	if event.Ref == "" {
		return
	}
	for i := len(m.transcript) - 1; i >= 0; i-- {
		entry := &m.transcript[i]
		if entry.Kind != "permission" || entry.ID != event.Ref {
			continue
		}
		entry.Status = event.Status
		if event.Text != "" {
			if entry.Detail != "" {
				entry.Detail += " | " + event.Text
			} else {
				entry.Detail = event.Text
			}
		}
		return
	}
}

func (m *model) toggleTranscriptBlockAt(x, y int) bool {
	if !m.transcriptRect.contains(x, y) {
		return false
	}
	line := m.transcriptVP.YOffset + (y - m.transcriptRect.y) + 1
	groupID := m.transcriptToggles[line]
	if groupID == "" {
		return false
	}
	m.collapsedExecs[groupID] = !m.executionCollapsed(groupID)
	return true
}

func (m *model) executionCollapsed(groupID string) bool {
	if collapsed, ok := m.collapsedExecs[groupID]; ok {
		return collapsed
	}
	for _, item := range m.transcript {
		if m.executionGroupID(item) != groupID {
			continue
		}
		if item.Status == "running" || item.Status == "blocked" || item.Status == "error" {
			return false
		}
	}
	return true
}

func (m *model) isWorking() bool {
	return m.pendingPrompt || m.status == "sending" || m.status == "running" || m.status == "blocked"
}

func (m *model) scrollViewportAt(x, y int, up bool) {
	switch {
	case m.transcriptRect.contains(x, y):
		if up {
			m.transcriptVP.LineUp(3)
		} else {
			m.transcriptVP.LineDown(3)
		}
	case m.infoRect.contains(x, y):
		if up {
			m.infoVP.LineUp(3)
		} else {
			m.infoVP.LineDown(3)
		}
	case m.activityRect.contains(x, y):
		if up {
			m.activityVP.LineUp(3)
		} else {
			m.activityVP.LineDown(3)
		}
	}
}

func slashCommandHint(value string) string {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "/") {
		return ""
	}
	commands := []string{
		"/new",
		"/resume",
		"/sessions",
		"/model",
		"/plan",
		"/build",
		"/mode build|plan",
		"/approvals conservative|allow-all",
		"/clear",
		"/quit",
		"/help",
	}
	if trimmed == "/" {
		return strings.Join(commands, "  ")
	}
	var matches []string
	for _, command := range commands {
		if strings.HasPrefix(command, trimmed) {
			matches = append(matches, command)
		}
	}
	return strings.Join(matches, "  ")
}
