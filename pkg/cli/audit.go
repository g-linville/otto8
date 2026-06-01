package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	gptcmd "github.com/gptscript-ai/cmd"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"
)

type Audit struct {
	root *Obot
}

func (a *Audit) Customize(cmd *cobra.Command) {
	cmd.Use = "audit"
	cmd.Short = "Submit local audit events to Obot"
	cmd.Args = cobra.NoArgs
	cmd.AddCommand(gptcmd.Command(&AuditClaudeCodeHook{root: a.root}))
}

func (a *Audit) Run(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

type AuditClaudeCodeHook struct {
	root *Obot
}

func (a *AuditClaudeCodeHook) Customize(cmd *cobra.Command) {
	cmd.Use = "claude-code-hook"
	cmd.Short = "Submit a Claude Code hook event as a local tool audit log"
	cmd.Args = cobra.NoArgs
}

func (a *AuditClaudeCodeHook) Run(cmd *cobra.Command, _ []string) error {
	entry, err := readClaudeCodeHookAudit(cmd.InOrStdin())
	if err != nil {
		return err
	}
	if a.root == nil || a.root.Client == nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "submit Claude Code audit log: no API client configured")
		return nil
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := a.root.Client.SubmitLocalAgentAuditLog(ctx, entry); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "submit Claude Code audit log: %v\n", err)
		return nil
	}
	return nil
}

type claudeCodeHookPayload struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	CWD            string          `json:"cwd"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolResponse   json.RawMessage `json:"tool_response"`
	ToolOutput     json.RawMessage `json:"tool_output"`
	ToolUseID      string          `json:"tool_use_id"`
	DurationMs     int64           `json:"duration_ms"`
	Error          string          `json:"error"`
}

func readClaudeCodeHookAudit(r io.Reader) (types.LocalAgentAuditLog, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return types.LocalAgentAuditLog{}, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return types.LocalAgentAuditLog{}, fmt.Errorf("Claude Code hook payload is empty")
	}

	var payload claudeCodeHookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return types.LocalAgentAuditLog{}, fmt.Errorf("parse Claude Code hook payload: %w", err)
	}
	switch payload.HookEventName {
	case "PostToolUse", "PostToolUseFailure":
	default:
		return types.LocalAgentAuditLog{}, fmt.Errorf("unsupported Claude Code hook event %q", payload.HookEventName)
	}
	if strings.TrimSpace(payload.ToolName) == "" {
		return types.LocalAgentAuditLog{}, fmt.Errorf("Claude Code hook payload missing tool_name")
	}

	response := payload.ToolResponse
	if len(response) == 0 {
		response = payload.ToolOutput
	}

	return types.LocalAgentAuditLog{
		Source:         "claude-code",
		EventName:      payload.HookEventName,
		ToolName:       payload.ToolName,
		SessionID:      payload.SessionID,
		ToolUseID:      payload.ToolUseID,
		CWD:            payload.CWD,
		TranscriptPath: payload.TranscriptPath,
		DurationMs:     payload.DurationMs,
		Success:        payload.HookEventName == "PostToolUse",
		Error:          payload.Error,
		ToolInput:      payload.ToolInput,
		ToolResponse:   response,
		Raw:            json.RawMessage(data),
		CreatedAt:      *types.NewTime(time.Now().UTC()),
	}, nil
}
