package localagents

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

const MaxAuditEventBytes = 2 * 1024 * 1024

const (
	AuditClientClaudeCode = "claude-code"
	AuditClientCodexCLI   = "codex-cli"
	AuditClientCursor     = "cursor"
)

type AuditNormalizeOptions struct {
	ClientID        string
	HookEvent       string
	Payload         []byte
	InputTruncated  bool
	MaxPayloadBytes int
}

type claudeCodeHookEvent struct {
	SessionID        string          `json:"session_id"`
	TranscriptPath   string          `json:"transcript_path"`
	CWD              string          `json:"cwd"`
	HookEventName    string          `json:"hook_event_name"`
	ToolName         string          `json:"tool_name"`
	ToolInput        json.RawMessage `json:"tool_input"`
	ToolResponse     json.RawMessage `json:"tool_response"`
	Error            json.RawMessage `json:"error"`
	PayloadTruncated bool            `json:"payload_truncated"`
}

type codexCLIHookEvent struct {
	SessionID        string          `json:"session_id"`
	TranscriptPath   string          `json:"transcript_path"`
	CWD              string          `json:"cwd"`
	HookEventName    string          `json:"hook_event_name"`
	Model            string          `json:"model"`
	PermissionMode   string          `json:"permission_mode"`
	TurnID           string          `json:"turn_id"`
	ToolName         string          `json:"tool_name"`
	ToolUseID        string          `json:"tool_use_id"`
	ToolInput        json.RawMessage `json:"tool_input"`
	ToolResponse     json.RawMessage `json:"tool_response"`
	PayloadTruncated bool            `json:"payload_truncated"`
}

type cursorHookEvent struct {
	SessionID        string          `json:"session_id"`
	ConversationID   string          `json:"conversationId"`
	RequestID        string          `json:"requestId"`
	CWD              string          `json:"cwd"`
	Workspace        cursorWorkspace `json:"workspace"`
	HookEventName    string          `json:"hookEventName"`
	ToolName         string          `json:"toolName"`
	ToolType         string          `json:"toolType"`
	Args             json.RawMessage `json:"args"`
	Input            json.RawMessage `json:"input"`
	Command          json.RawMessage `json:"command"`
	Result           json.RawMessage `json:"result"`
	Output           json.RawMessage `json:"output"`
	Stdout           json.RawMessage `json:"stdout"`
	Stderr           json.RawMessage `json:"stderr"`
	Error            json.RawMessage `json:"error"`
	Status           string          `json:"status"`
	ExitCode         *int            `json:"exitCode"`
	DurationMs       *int64          `json:"duration_ms"`
	PayloadTruncated bool            `json:"payloadTruncated"`
}

type cursorWorkspace struct {
	Name string `json:"name"`
}

type toolResponseMetadata struct {
	Status   string          `json:"status"`
	ExitCode *int            `json:"exit_code"`
	Error    json.RawMessage `json:"error"`
}

type errorMetadata struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Stderr  string `json:"stderr"`
}

func NormalizeAuditEvent(opts AuditNormalizeOptions) (types.LocalAgentAuditLogIngest, error) {
	clientID := strings.TrimSpace(opts.ClientID)
	if clientID == "" {
		return types.LocalAgentAuditLogIngest{}, fmt.Errorf("client is required")
	}
	hookEvent := strings.TrimSpace(opts.HookEvent)
	if hookEvent == "" {
		return types.LocalAgentAuditLogIngest{}, fmt.Errorf("event is required")
	}

	maxBytes := opts.MaxPayloadBytes
	if maxBytes <= 0 {
		maxBytes = MaxAuditEventBytes
	}

	event := types.LocalAgentAuditLogIngest{
		LocalAgentAuditLogFields: types.LocalAgentAuditLogFields{
			EventID:   stableAuditEventID(clientID, hookEvent, opts.Payload, opts.InputTruncated),
			EventName: hookEvent,
		},
		Client:           types.LocalAgentAuditLogIngestClient{Name: clientID},
		PayloadTruncated: opts.InputTruncated,
	}

	payload := bytes.TrimSpace(opts.Payload)
	if len(payload) == 0 {
		return clampAuditEventPayload(event, maxBytes), nil
	}

	var err error
	switch clientID {
	case AuditClientClaudeCode:
		var native claudeCodeHookEvent
		err = decodeAuditHookPayload(payload, &native)
		if err == nil {
			event = native.toAuditLog(event, payload)
		}
	case AuditClientCodexCLI:
		var native codexCLIHookEvent
		err = decodeAuditHookPayload(payload, &native)
		if err == nil {
			event = native.toAuditLog(event, payload)
		}
	case AuditClientCursor:
		var native cursorHookEvent
		err = decodeAuditHookPayload(payload, &native)
		if err == nil {
			event = native.toAuditLog(event, payload)
		}
	default:
		err = fmt.Errorf("unsupported client %q", clientID)
	}
	if err != nil {
		if opts.InputTruncated {
			return clampAuditEventPayload(event, maxBytes), nil
		}
		return types.LocalAgentAuditLogIngest{}, err
	}

	return clampAuditEventPayload(event, maxBytes), nil
}

func decodeAuditHookPayload(payload []byte, target any) error {
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("read hook JSON: %w", err)
	}
	return nil
}

func (e claudeCodeHookEvent) toAuditLog(event types.LocalAgentAuditLogIngest, payload []byte) types.LocalAgentAuditLogIngest {
	event.RawClientHookEvent = copyRaw(payload)
	event.EventName = firstString(e.HookEventName, event.EventName)
	event.SessionID = e.SessionID
	event.ConversationID = basenameNoExt(e.TranscriptPath)
	event.ToolName = e.ToolName
	event.ToolType = inferToolType(e.ToolName)
	event.RawToolInput = copyRaw(e.ToolInput)
	event.RawToolOutput = copyRaw(e.ToolResponse)
	event.RawError = copyRaw(e.Error)
	event.PayloadTruncated = event.PayloadTruncated || e.PayloadTruncated
	applyWorkspace(&event, e.CWD, "")

	response := parseToolResponseMetadata(e.ToolResponse)
	event.Status = firstString(response.Status, errorType(e.Error))
	event.ExitCode = response.ExitCode
	event.Error = normalizedErrorMessage(e.Error, nil)
	applyOutcome(&event)
	return event
}

func (e codexCLIHookEvent) toAuditLog(event types.LocalAgentAuditLogIngest, payload []byte) types.LocalAgentAuditLogIngest {
	event.RawClientHookEvent = copyRaw(payload)
	event.EventName = firstString(e.HookEventName, event.EventName)
	event.SessionID = e.SessionID
	event.ConversationID = firstString(e.TurnID, basenameNoExt(e.TranscriptPath))
	event.RequestID = e.ToolUseID
	event.ToolName = e.ToolName
	event.ToolType = inferToolType(e.ToolName)
	event.RawToolInput = copyRaw(e.ToolInput)
	event.RawToolOutput = copyRaw(e.ToolResponse)
	event.PayloadTruncated = event.PayloadTruncated || e.PayloadTruncated
	applyWorkspace(&event, e.CWD, "")

	response := parseToolResponseMetadata(e.ToolResponse)
	event.Status = response.Status
	event.ExitCode = response.ExitCode
	event.RawError = copyRaw(response.Error)
	event.Error = normalizedErrorMessage(response.Error, nil)
	applyOutcome(&event)
	return event
}

func (e cursorHookEvent) toAuditLog(event types.LocalAgentAuditLogIngest, payload []byte) types.LocalAgentAuditLogIngest {
	event.RawClientHookEvent = copyRaw(payload)
	event.EventName = firstString(e.HookEventName, event.EventName)
	event.SessionID = e.SessionID
	event.ConversationID = e.ConversationID
	event.RequestID = e.RequestID
	event.ToolName = e.ToolName
	event.ToolType = firstString(e.ToolType, inferToolType(e.ToolName), inferToolType(e.HookEventName))
	event.DurationMs = e.DurationMs
	event.ExitCode = e.ExitCode
	event.Status = e.Status
	event.PayloadTruncated = event.PayloadTruncated || e.PayloadTruncated
	event.RawToolInput = firstRaw(e.Args, e.Input, e.Command)
	event.RawToolOutput = firstRaw(e.Result, e.Output, cursorOutputRaw(e.Stdout, e.Stderr))
	event.RawError = firstRaw(e.Error, cursorStderrRaw(e.Stderr))
	event.Error = normalizedErrorMessage(e.Error, e.Stderr)
	applyWorkspace(&event, e.CWD, e.Workspace.Name)
	applyOutcome(&event)
	return event
}

func applyWorkspace(event *types.LocalAgentAuditLogIngest, cwd, name string) {
	if cwd == "" {
		return
	}
	event.WorkspaceHash = "sha256:" + hashString(cwd)
	event.WorkspaceBasename = firstString(name, filepath.Base(cwd))
}

func applyOutcome(event *types.LocalAgentAuditLogIngest) {
	success := inferSuccess(event.EventName, event.Status, event.ExitCode, event.RawError)
	if success == nil && len(event.RawToolOutput) > 0 && len(event.RawError) == 0 {
		v := true
		success = &v
	}
	event.Success = success
	if event.Status == "" && success != nil {
		if *success {
			event.Status = "success"
		} else {
			event.Status = "failure"
		}
	}
}

func clampAuditEventPayload(event types.LocalAgentAuditLogIngest, maxBytes int) types.LocalAgentAuditLogIngest {
	if auditEventFits(event, maxBytes) {
		return event
	}
	event.PayloadTruncated = true
	for _, drop := range []func(*types.LocalAgentAuditLogIngest){
		func(e *types.LocalAgentAuditLogIngest) { e.RawToolOutput = nil },
		func(e *types.LocalAgentAuditLogIngest) { e.RawToolInput = nil },
		func(e *types.LocalAgentAuditLogIngest) { e.RawError = nil },
		func(e *types.LocalAgentAuditLogIngest) { e.RawClientHookEvent = nil },
	} {
		drop(&event)
		if auditEventFits(event, maxBytes) {
			return event
		}
	}
	return event
}

func auditEventFits(event types.LocalAgentAuditLogIngest, maxBytes int) bool {
	data, err := json.Marshal(event)
	return err != nil || len(data) <= maxBytes
}

func stableAuditEventID(clientID, hookEvent string, payload []byte, truncated bool) string {
	hash := sha256.New()
	hash.Write([]byte(clientID))
	hash.Write([]byte{0})
	hash.Write([]byte(hookEvent))
	hash.Write([]byte{0})
	hash.Write(payload)
	if truncated {
		hash.Write([]byte{0, 1})
	}
	return "local-agent-" + hex.EncodeToString(hash.Sum(nil))
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func parseToolResponseMetadata(raw json.RawMessage) toolResponseMetadata {
	var response toolResponseMetadata
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &response)
	}
	return response
}

func normalizedErrorMessage(rawError, rawStderr json.RawMessage) string {
	if len(rawError) > 0 {
		var payload errorMetadata
		if err := json.Unmarshal(rawError, &payload); err == nil {
			if strings.TrimSpace(payload.Message) != "" {
				return strings.TrimSpace(payload.Message)
			}
			if strings.TrimSpace(payload.Stderr) != "" {
				return strings.TrimSpace(payload.Stderr)
			}
		}
	}
	if stderr := rawString(rawStderr); stderr != "" {
		return stderr
	}
	return ""
}

func errorType(rawError json.RawMessage) string {
	var payload errorMetadata
	if len(rawError) > 0 {
		_ = json.Unmarshal(rawError, &payload)
	}
	return payload.Type
}

func inferSuccess(eventName, status string, exitCode *int, rawError json.RawMessage) *bool {
	lowerEvent := strings.ToLower(eventName)
	lowerStatus := strings.ToLower(status)
	if strings.Contains(lowerEvent, "failure") || strings.Contains(lowerStatus, "fail") ||
		strings.Contains(lowerStatus, "error") || strings.Contains(lowerStatus, "timeout") ||
		strings.Contains(lowerStatus, "timed_out") || strings.Contains(lowerStatus, "cancel") {
		v := false
		return &v
	}
	if exitCode != nil {
		v := *exitCode == 0
		return &v
	}
	if len(rawError) > 0 {
		v := false
		return &v
	}
	if strings.Contains(lowerStatus, "success") || strings.Contains(lowerStatus, "complete") ||
		strings.Contains(lowerStatus, "ok") {
		v := true
		return &v
	}
	return nil
}

func inferToolType(toolName string) string {
	lower := strings.ToLower(toolName)
	switch {
	case lower == "bash" || lower == "shell" || strings.Contains(lower, "shell"):
		return "shell"
	case strings.HasPrefix(lower, "mcp__"):
		return "mcp"
	case strings.Contains(lower, "edit") || strings.Contains(lower, "write") || strings.Contains(lower, "apply_patch"):
		return "file"
	default:
		return ""
	}
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstRaw(values ...json.RawMessage) json.RawMessage {
	for _, value := range values {
		if len(bytes.TrimSpace(value)) > 0 && string(bytes.TrimSpace(value)) != "null" {
			return copyRaw(value)
		}
	}
	return nil
}

func copyRaw(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return json.RawMessage(append([]byte(nil), raw...))
}

func cursorOutputRaw(stdout, stderr json.RawMessage) json.RawMessage {
	output := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(stdout)) > 0 {
		output["stdout"] = copyRaw(stdout)
	}
	if len(bytes.TrimSpace(stderr)) > 0 {
		output["stderr"] = copyRaw(stderr)
	}
	if len(output) == 0 {
		return nil
	}
	return mustRaw(output)
}

func cursorStderrRaw(stderr json.RawMessage) json.RawMessage {
	if rawString(stderr) == "" {
		return nil
	}
	return mustRaw(map[string]json.RawMessage{"stderr": stderr})
}

func rawString(raw json.RawMessage) string {
	var value string
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &value)
	}
	return strings.TrimSpace(value)
}

func mustRaw(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

func basenameNoExt(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
