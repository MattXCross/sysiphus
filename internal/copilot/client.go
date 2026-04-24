package copilotbridge

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/matt/sysiphus/internal/agents"
	"github.com/matt/sysiphus/internal/state"
)

var errCopilotMissing = errors.New("copilot CLI is not installed or not on PATH")

type Runtime struct {
	mu             sync.Mutex
	ctx            context.Context
	client         *copilot.Client
	session        *copilot.Session
	events         chan AppEvent
	cwd            string
	mode           string
	approvalPolicy state.ApprovalPolicy
	sessionID      string
}

type AppEvent struct {
	Kind    string
	Ref     string
	Text    string
	Detail  string
	Status  string
	Session string
}

func NewRuntime() *Runtime {
	return &Runtime{
		events:         make(chan AppEvent, 512),
		mode:           agents.ModeBuild,
		approvalPolicy: state.ApprovalConservative,
	}
}

func (r *Runtime) Events() <-chan AppEvent {
	return r.events
}

func (r *Runtime) SetMode(mode string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if mode == agents.ModePlan || mode == agents.ModeBuild {
		r.mode = mode
	}
}

func (r *Runtime) SetApprovalPolicy(policy state.ApprovalPolicy) {
	r.mu.Lock()
	r.approvalPolicy = policy
	r.mu.Unlock()
}

func (r *Runtime) Mode() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mode
}

func (r *Runtime) ApprovalPolicy() state.ApprovalPolicy {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.approvalPolicy
}

func (r *Runtime) Start(ctx context.Context, cwd string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		return nil
	}
	if _, err := exec.LookPath("copilot"); err != nil {
		return errCopilotMissing
	}
	client := copilot.NewClient(&copilot.ClientOptions{
		Cwd:      cwd,
		LogLevel: "error",
	})
	if err := client.Start(ctx); err != nil {
		return err
	}
	r.ctx = ctx
	r.cwd = cwd
	r.client = client
	r.publish("status", "Connected to Copilot runtime", "", "ready", "")
	return nil
}

func (r *Runtime) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var errs []string
	if r.session != nil {
		if err := r.session.Disconnect(); err != nil {
			errs = append(errs, err.Error())
		}
		r.session = nil
	}
	if r.client != nil {
		if err := r.client.Stop(); err != nil {
			errs = append(errs, err.Error())
		}
		r.client = nil
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (r *Runtime) SessionID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessionID
}

func (r *Runtime) NewSession(ctx context.Context, cwd string, model string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client == nil {
		return "", errors.New("runtime not started")
	}
	resolvedModel, err := r.resolveModelLocked(ctx, model)
	if err != nil {
		return "", err
	}
	if r.session != nil {
		_ = r.session.Disconnect()
		r.session = nil
	}
	mode := r.mode
	policy := r.approvalPolicy
	sess, err := r.client.CreateSession(ctx, &copilot.SessionConfig{
		ClientName:          "sysiphus",
		Model:               resolvedModel,
		WorkingDirectory:    cwd,
		Streaming:           true,
		OnPermissionRequest: approvalHandler(policy),
		CustomAgents:        agents.Builtins(),
		Agent:               mode,
		OnEvent:             r.handleEvent,
	})
	if err != nil {
		return "", err
	}
	r.session = sess
	r.sessionID = sess.SessionID
	r.publish("status", fmt.Sprintf("Started session with model %s", resolvedModel), sess.SessionID, "ready", "")
	return sess.SessionID, nil
}

func (r *Runtime) ResumeLatest(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client == nil {
		return "", errors.New("runtime not started")
	}
	id, err := r.client.GetLastSessionID(ctx)
	if err != nil {
		return "", err
	}
	if id == nil || *id == "" {
		return "", errors.New("no previous session found")
	}
	if r.session != nil {
		_ = r.session.Disconnect()
		r.session = nil
	}
	sess, err := r.client.ResumeSession(ctx, *id, &copilot.ResumeSessionConfig{
		Streaming:           true,
		OnPermissionRequest: approvalHandler(r.approvalPolicy),
	})
	if err != nil {
		return "", err
	}
	sess.On(r.handleEvent)
	r.session = sess
	r.sessionID = sess.SessionID
	r.publish("status", "Resumed session", sess.SessionID, "ready", "")
	return sess.SessionID, nil
}

func (r *Runtime) ListSessions(ctx context.Context) ([]state.SessionSummary, error) {
	r.mu.Lock()
	client := r.client
	r.mu.Unlock()
	if client == nil {
		return nil, errors.New("runtime not started")
	}
	items, err := client.ListSessions(ctx, nil)
	if err != nil {
		return nil, err
	}
	result := make([]state.SessionSummary, 0, len(items))
	for _, item := range items {
		updatedAt := parseSDKTime(item.ModifiedTime)
		title := ""
		if item.Summary != nil {
			title = *item.Summary
		}
		result = append(result, state.SessionSummary{
			ID:        item.SessionID,
			Title:     title,
			UpdatedAt: updatedAt,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

func (r *Runtime) ListModels(ctx context.Context) ([]state.ModelOption, error) {
	r.mu.Lock()
	client := r.client
	r.mu.Unlock()
	if client == nil {
		return nil, errors.New("runtime not started")
	}
	models, err := client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]state.ModelOption, 0, len(models))
	for _, model := range models {
		result = append(result, state.ModelOption{
			ID:                model.ID,
			Name:              model.Name,
			SupportsReasoning: model.Capabilities.Supports.ReasoningEffort,
			DefaultReasoning:  model.DefaultReasoningEffort,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func (r *Runtime) SwitchModel(ctx context.Context, modelID string) error {
	r.mu.Lock()
	sess := r.session
	r.mu.Unlock()
	if sess == nil {
		return errors.New("no active session")
	}
	if err := sess.SetModel(ctx, modelID, nil); err != nil {
		return err
	}
	r.publish("status", fmt.Sprintf("Switching model to %s", modelID), "", "running", "")
	return nil
}

func (r *Runtime) Send(ctx context.Context, prompt string) error {
	r.mu.Lock()
	sess := r.session
	r.mu.Unlock()
	if sess == nil {
		return errors.New("no active session")
	}
	_, err := sess.Send(ctx, copilot.MessageOptions{Prompt: prompt})
	return err
}

func approvalHandler(policy state.ApprovalPolicy) copilot.PermissionHandlerFunc {
	return func(req copilot.PermissionRequest, _ copilot.PermissionInvocation) (copilot.PermissionRequestResult, error) {
		if policy == state.ApprovalAllowAll {
			return copilot.PermissionRequestResult{Kind: copilot.PermissionRequestResultKindApproved}, nil
		}
		switch req.Kind {
		case copilot.PermissionRequestKindRead,
			copilot.PermissionRequestKindWrite,
			copilot.PermissionRequestKindMemory,
			copilot.PermissionRequestKindCustomTool,
			copilot.PermissionRequestKindMcp,
			copilot.PermissionRequestKindURL:
			return copilot.PermissionRequestResult{Kind: copilot.PermissionRequestResultKindApproved}, nil
		case copilot.PermissionRequestKindShell:
			if !isHighRiskShellCommand(req.FullCommandText) {
				return copilot.PermissionRequestResult{Kind: copilot.PermissionRequestResultKindApproved}, nil
			}
		}
		return copilot.PermissionRequestResult{Kind: copilot.PermissionRequestResultKindDeniedInteractivelyByUser}, nil
	}
}

func (r *Runtime) handleEvent(event copilot.SessionEvent) {
	switch data := event.Data.(type) {
	case *copilot.UserMessageData:
		r.publish("user", data.Content, "", "ready", "")
	case *copilot.AssistantReasoningDeltaData:
		r.publish("reasoning_delta", data.DeltaContent, "", "running", data.ReasoningID)
	case *copilot.AssistantReasoningData:
		r.publish("reasoning", data.Content, "", "running", data.ReasoningID)
	case *copilot.AssistantMessageDeltaData:
		r.publish("assistant_delta", data.DeltaContent, "", "running", data.MessageID)
	case *copilot.AssistantMessageData:
		r.publish("assistant", data.Content, "", "ready", data.MessageID)
	case *copilot.AssistantIntentData:
		r.publish("intent", data.Intent, "", "running", "")
	case *copilot.ToolExecutionStartData:
		r.publish("tool", data.ToolName, formatToolDetail(data.Arguments, data.McpServerName), "running", data.ToolCallID)
	case *copilot.ToolExecutionProgressData:
		r.publish("tool_progress", data.ProgressMessage, "", "running", data.ToolCallID)
	case *copilot.ToolExecutionCompleteData:
		text := "completed"
		detail := ""
		if data.Result != nil && data.Result.DetailedContent != nil {
			detail = *data.Result.DetailedContent
		}
		if data.Error != nil && data.Error.Message != "" {
			detail = data.Error.Message
		}
		status := "ready"
		if !data.Success {
			text = "failed"
			status = "error"
		}
		r.publish("tool_result", text, detail, status, data.ToolCallID)
	case *copilot.PermissionRequestedData:
		r.publish("permission", fmt.Sprintf("requested %s permission", data.PermissionRequest.Kind), describePermission(data.PermissionRequest), "blocked", requestRef(data.PermissionRequest.ToolCallID))
	case *copilot.PermissionCompletedData:
		r.publish("permission_result", fmt.Sprintf("permission %s", data.Result.Kind), data.RequestID, permissionStatus(data.Result.Kind), "")
	case *copilot.SubagentSelectedData:
		r.publish("subagent", fmt.Sprintf("selected %s", data.AgentDisplayName), strings.Join(data.Tools, ", "), "running", data.AgentName)
	case *copilot.SubagentStartedData:
		r.publish("subagent", fmt.Sprintf("%s started", data.AgentDisplayName), data.AgentDescription, "running", data.ToolCallID)
	case *copilot.SubagentCompletedData:
		r.publish("subagent_result", fmt.Sprintf("%s completed", data.AgentDisplayName), describeSubagentTotals(data.Model, data.TotalToolCalls, data.TotalTokens, data.DurationMs), "ready", data.ToolCallID)
	case *copilot.SubagentFailedData:
		r.publish("subagent_failed", fmt.Sprintf("%s failed", data.AgentDisplayName), joinNonEmpty([]string{data.Error, describeSubagentTotals(data.Model, data.TotalToolCalls, data.TotalTokens, data.DurationMs)}, " | "), "error", data.ToolCallID)
	case *copilot.SessionContextChangedData:
		branch := ""
		if data.Branch != nil {
			branch = *data.Branch
		}
		r.publish("context", data.Cwd, branch, "ready", "")
	case *copilot.SessionUsageInfoData:
		r.publish("usage", fmt.Sprintf("%.0f / %.0f tokens", data.CurrentTokens, data.TokenLimit), fmt.Sprintf("messages %.0f", data.MessagesLength), "ready", "")
	case *copilot.SessionModelChangeData:
		reasoning := ""
		if data.ReasoningEffort != nil {
			reasoning = *data.ReasoningEffort
		}
		r.publish("model", data.NewModel, reasoning, "ready", "")
	case *copilot.SessionErrorData:
		r.publish("error", data.Message, "", "error", "")
	case *copilot.SessionIdleData:
		r.publish("idle", "Session idle", "", "ready", "")
	}
}

func describePermission(req copilot.PermissionRequestedDataPermissionRequest) string {
	parts := []string{string(req.Kind)}
	if req.Intention != nil && *req.Intention != "" {
		parts = append(parts, *req.Intention)
	}
	if req.FullCommandText != nil && *req.FullCommandText != "" {
		parts = append(parts, *req.FullCommandText)
	}
	if req.FileName != nil && *req.FileName != "" {
		parts = append(parts, *req.FileName)
	}
	if req.Path != nil && *req.Path != "" {
		parts = append(parts, *req.Path)
	}
	if req.URL != nil && *req.URL != "" {
		parts = append(parts, *req.URL)
	}
	return strings.Join(parts, " | ")
}

func (r *Runtime) publish(kind, text, detail, status, ref string) {
	select {
	case r.events <- AppEvent{Kind: kind, Ref: ref, Text: text, Detail: detail, Status: status, Session: r.sessionID}:
	default:
		go func() {
			select {
			case r.events <- AppEvent{Kind: kind, Ref: ref, Text: text, Detail: detail, Status: status, Session: r.sessionID}:
			case <-time.After(50 * time.Millisecond):
			}
		}()
	}
}

func permissionStatus(kind copilot.PermissionCompletedDataResultKind) string {
	if strings.Contains(strings.ToLower(string(kind)), "denied") {
		return "blocked"
	}
	return "ready"
}

func requestRef(id *string) string {
	if id == nil {
		return ""
	}
	return *id
}

func formatToolDetail(args any, mcpServer *string) string {
	parts := []string{}
	if mcpServer != nil && *mcpServer != "" {
		parts = append(parts, fmt.Sprintf("server %s", *mcpServer))
	}
	if args != nil {
		parts = append(parts, fmt.Sprint(args))
	}
	return joinNonEmpty(parts, " | ")
}

func describeSubagentTotals(model *string, totalToolCalls, totalTokens, durationMs *float64) string {
	parts := []string{}
	if model != nil && *model != "" {
		parts = append(parts, fmt.Sprintf("model %s", *model))
	}
	if totalToolCalls != nil {
		parts = append(parts, fmt.Sprintf("tools %.0f", *totalToolCalls))
	}
	if totalTokens != nil {
		parts = append(parts, fmt.Sprintf("tokens %.0f", *totalTokens))
	}
	if durationMs != nil {
		parts = append(parts, fmt.Sprintf("%.0fms", *durationMs))
	}
	return joinNonEmpty(parts, "  ")
}

func joinNonEmpty(items []string, sep string) string {
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		if item != "" {
			filtered = append(filtered, item)
		}
	}
	return strings.Join(filtered, sep)
}

func isHighRiskShellCommand(command *string) bool {
	if command == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(*command))
	highRiskFragments := []string{
		"sudo ",
		" su ",
		"doas ",
		"rm -rf /",
		"rm -rf ~",
		"mkfs",
		"dd if=",
		"shutdown",
		"reboot",
		"poweroff",
		"halt",
		"systemctl",
		"init 0",
		"init 6",
		"chmod ",
		"chown ",
		"mount ",
		"umount ",
		"useradd",
		"userdel",
		"groupadd",
		"groupdel",
		"passwd ",
	}
	for _, fragment := range highRiskFragments {
		if strings.Contains(text, fragment) {
			return true
		}
	}
	return false
}

func parseSDKTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts
	}
	return time.Time{}
}

func (r *Runtime) resolveModelLocked(ctx context.Context, requested string) (string, error) {
	models, err := r.client.ListModels(ctx)
	if err != nil {
		if requested != "" {
			return requested, nil
		}
		return "", err
	}
	if len(models) == 0 {
		if requested != "" {
			return requested, nil
		}
		return "", errors.New("no Copilot models available")
	}
	if requested != "" {
		for _, model := range models {
			if model.ID == requested {
				return requested, nil
			}
		}
	}
	preferred := []string{"gpt-5", "gpt-4.1", "claude-sonnet-4.5", "claude-3.7-sonnet", "gemini-2.5-pro"}
	for _, want := range preferred {
		for _, model := range models {
			if model.ID == want {
				return model.ID, nil
			}
		}
	}
	return models[0].ID, nil
}
