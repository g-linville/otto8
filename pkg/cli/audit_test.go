package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"
)

func TestReadClaudeCodeHookAuditSuccess(t *testing.T) {
	entry, err := readClaudeCodeHookAudit(strings.NewReader(`{
  "session_id": "session-1",
  "transcript_path": "/tmp/transcript.jsonl",
  "cwd": "/repo",
  "hook_event_name": "PostToolUse",
  "tool_name": "Bash",
  "tool_use_id": "toolu_1",
  "duration_ms": 42,
  "tool_input": {"command": "pwd"},
  "tool_response": {"stdout": "/repo\n"}
}`))
	if err != nil {
		t.Fatal(err)
	}
	if entry.Source != "claude-code" || entry.ToolName != "Bash" || !entry.Success {
		t.Fatalf("unexpected entry: %#v", entry)
	}
	if entry.SessionID != "session-1" || entry.ToolUseID != "toolu_1" || entry.DurationMs != 42 {
		t.Fatalf("unexpected metadata: %#v", entry)
	}
	if !json.Valid(entry.ToolInput) || !json.Valid(entry.ToolResponse) || !json.Valid(entry.Raw) {
		t.Fatalf("expected valid raw JSON fields: %#v", entry)
	}
}

func TestReadClaudeCodeHookAuditFailure(t *testing.T) {
	entry, err := readClaudeCodeHookAudit(strings.NewReader(`{
  "hook_event_name": "PostToolUseFailure",
  "tool_name": "Read",
  "tool_use_id": "toolu_2",
  "duration_ms": 7,
  "tool_input": {"file_path": "/missing"},
  "error": "file not found"
}`))
	if err != nil {
		t.Fatal(err)
	}
	if entry.Success {
		t.Fatalf("expected failure entry")
	}
	if entry.Error != "file not found" {
		t.Fatalf("Error = %q, want file not found", entry.Error)
	}
}

func TestReadClaudeCodeHookAuditRejectsInvalidJSON(t *testing.T) {
	if _, err := readClaudeCodeHookAudit(strings.NewReader(`{`)); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestAuditClaudeCodeHookSubmitsPayload(t *testing.T) {
	var got types.LocalAgentAuditLog
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/local-agent-audit-logs" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(types.LocalAgentAuditLogResponse{Accepted: true})
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"pwd"}}`))
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	hook := &AuditClaudeCodeHook{root: &Obot{Client: &apiclient.Client{BaseURL: server.URL + "/api", Token: "token"}}}
	if err := hook.Run(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	if got.Source != "claude-code" || got.ToolName != "Bash" || !got.Success {
		t.Fatalf("unexpected submitted entry: %#v", got)
	}
}

func TestAuditClaudeCodeHookSubmitFailureIsNonBlocking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"PostToolUse","tool_name":"Bash"}`))
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	hook := &AuditClaudeCodeHook{root: &Obot{Client: &apiclient.Client{BaseURL: server.URL + "/api", Token: "token"}}}
	if err := hook.Run(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "submit Claude Code audit log") {
		t.Fatalf("expected submit warning, got %q", stderr.String())
	}
}
