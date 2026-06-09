package localagents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeAuditEventFixtures(t *testing.T) {
	tests := []struct {
		name        string
		client      string
		event       string
		path        string
		wantEvent   string
		wantTool    string
		wantType    string
		wantSuccess bool
	}{
		{
			name:        "claude success",
			client:      "claude-code",
			event:       "post-tool-use",
			path:        "claude-code/post-tool-use-success.json",
			wantEvent:   "PostToolUse",
			wantTool:    "Bash",
			wantType:    "shell",
			wantSuccess: true,
		},
		{
			name:        "claude failure",
			client:      "claude-code",
			event:       "post-tool-use-failure",
			path:        "claude-code/post-tool-use-failure.json",
			wantEvent:   "PostToolUseFailure",
			wantTool:    "Bash",
			wantType:    "shell",
			wantSuccess: false,
		},
		{
			name:        "codex failure",
			client:      "codex-cli",
			event:       "post-tool-use",
			path:        "codex-cli/post-tool-use-failure.json",
			wantEvent:   "PostToolUse",
			wantTool:    "Bash",
			wantType:    "shell",
			wantSuccess: false,
		},
		{
			name:        "codex timeout",
			client:      "codex-cli",
			event:       "post-tool-use",
			path:        "codex-cli/post-tool-use-timeout.json",
			wantEvent:   "PostToolUse",
			wantTool:    "Bash",
			wantType:    "shell",
			wantSuccess: false,
		},
		{
			name:        "cursor shell success",
			client:      "cursor",
			event:       "after-shell-execution",
			path:        "cursor/after-shell-execution-success.json",
			wantEvent:   "afterShellExecution",
			wantTool:    "shell",
			wantType:    "shell",
			wantSuccess: true,
		},
		{
			name:        "cursor mcp failure",
			client:      "cursor",
			event:       "post-tool-use-failure",
			path:        "cursor/post-tool-use-failure.json",
			wantEvent:   "postToolUseFailure",
			wantTool:    "mcp__filesystem__read_file",
			wantType:    "mcp",
			wantSuccess: false,
		},
		{
			name:        "cursor mcp oversized",
			client:      "cursor",
			event:       "after-mcp-execution",
			path:        "cursor/after-mcp-execution-oversized.json",
			wantEvent:   "afterMCPExecution",
			wantTool:    "mcp__filesystem__read_file",
			wantType:    "mcp",
			wantSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "audit-hooks", tt.path))
			if err != nil {
				t.Fatal(err)
			}
			got, err := NormalizeAuditEvent(AuditNormalizeOptions{
				ClientID:  tt.client,
				HookEvent: tt.event,
				Payload:   data,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.Client.Name != tt.client {
				t.Fatalf("client = %q, want %q", got.Client.Name, tt.client)
			}
			if got.EventName != tt.wantEvent {
				t.Fatalf("event = %q, want %q", got.EventName, tt.wantEvent)
			}
			if got.EventID == "" {
				t.Fatalf("event ID is empty")
			}
			if got.ToolName != tt.wantTool {
				t.Fatalf("tool = %q, want %q", got.ToolName, tt.wantTool)
			}
			if got.ToolType != tt.wantType {
				t.Fatalf("tool type = %q, want %q", got.ToolType, tt.wantType)
			}
			if got.Success == nil || *got.Success != tt.wantSuccess {
				t.Fatalf("success = %v, want %t", got.Success, tt.wantSuccess)
			}
			if got.WorkspaceHash == "" || strings.Contains(got.WorkspaceHash, "/Users/alex") {
				t.Fatalf("workspace hash should be present and not contain a path, got %q", got.WorkspaceHash)
			}
			if got.WorkspaceBasename != "obot" {
				t.Fatalf("workspace basename = %q, want obot", got.WorkspaceBasename)
			}
			if len(got.RawClientHookEvent) == 0 {
				t.Fatalf("raw client hook event is empty")
			}
			if got.RawToolInput == nil {
				t.Fatalf("raw tool input is empty")
			}
			if _, err := json.Marshal(got); err != nil {
				t.Fatalf("normalized event should marshal: %v", err)
			}
		})
	}
}

func TestNormalizeAuditEventOversizedInputFallsBackToMetadata(t *testing.T) {
	payload := []byte(`{"session_id":"partial"`)
	got, err := NormalizeAuditEvent(AuditNormalizeOptions{
		ClientID:       "cursor",
		HookEvent:      "after-shell-execution",
		Payload:        payload,
		InputTruncated: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.PayloadTruncated {
		t.Fatalf("payloadTruncated = false, want true")
	}
	if got.EventName != "after-shell-execution" {
		t.Fatalf("event = %q, want flag fallback", got.EventName)
	}
	if len(got.RawClientHookEvent) != 0 {
		t.Fatalf("raw client hook event should be dropped for unparseable oversized input")
	}
}

func TestNormalizeAuditEventClampsRawPayloadsToMaxBytes(t *testing.T) {
	payload := []byte(`{
		"session_id":"large",
		"cwd":"/Users/alex/src/obot",
		"hook_event_name":"PostToolUse",
		"tool_name":"Bash",
		"tool_input":{"command":"go test"},
		"tool_response":{"stdout":"` + strings.Repeat("x", 4096) + `"}
	}`)
	got, err := NormalizeAuditEvent(AuditNormalizeOptions{
		ClientID:        "claude-code",
		HookEvent:       "post-tool-use",
		Payload:         payload,
		MaxPayloadBytes: 512,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.PayloadTruncated {
		t.Fatalf("payloadTruncated = false, want true")
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 512 {
		t.Fatalf("normalized payload length = %d, want <= 512", len(data))
	}
	if got.ToolName != "Bash" || got.WorkspaceBasename != "obot" {
		t.Fatalf("metadata was not preserved: %#v", got)
	}
}
