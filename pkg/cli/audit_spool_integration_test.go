package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
	"github.com/obot-platform/obot/pkg/localagents"
)

func TestAuditCommandRealSpoolOfflineAppendAndDrainIntegration(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("real local audit spool integration test requires the macOS keychain")
	}

	unique := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	appURL := fmt.Sprintf("https://obot-spool-integration-%s.example.com", unique)
	apiBaseURL := localconfig.APIBaseURL(appURL)

	spool, err := newDefaultAuditSpool(appURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(spool.path)
		_ = os.Remove(spool.path + ".lock")
		_ = spool.keyStore.Delete(appURL)
	})

	secretMarker := "obot-spool-secret-" + unique
	outputMarker := "obot-spool-output-" + unique
	payload := claudeCodeAuditPayload(t, secretMarker, outputMarker)
	expected, err := localagents.NormalizeAuditEvent(localagents.AuditNormalizeOptions{
		ClientID:  localagents.AuditClientClaudeCode,
		HookEvent: "post-tool-use",
		Payload:   payload,
	})
	if err != nil {
		t.Fatal(err)
	}

	offlineAudit := Audit{
		Client:     localagents.AuditClientClaudeCode,
		Event:      "post-tool-use",
		root:       &Obot{Client: &apiclient.Client{BaseURL: apiBaseURL}},
		auditToken: func(string) (string, error) { return "audit-token", nil },
		submit: func(context.Context, *apiclient.Client, types.LocalAgentAuditLogIngest) error {
			return context.DeadlineExceeded
		},
	}
	if err := offlineAudit.Run(auditTestCommand(string(payload), nil, nil), nil); err != nil {
		t.Fatal(err)
	}

	spooled, err := os.ReadFile(spool.path)
	if err != nil {
		t.Fatalf("expected failed send to create real spool file: %v", err)
	}
	for _, plaintext := range []string{secretMarker, outputMarker, expected.EventID, "tool_input", "tool_response"} {
		if bytes.Contains(spooled, []byte(plaintext)) {
			t.Fatalf("real spool file contains plaintext %q: %s", plaintext, spooled)
		}
	}

	var submitted []types.LocalAgentAuditLogIngest
	onlineAudit := Audit{
		Client:     localagents.AuditClientClaudeCode,
		Event:      "post-tool-use",
		root:       &Obot{Client: &apiclient.Client{BaseURL: apiBaseURL}},
		auditToken: func(string) (string, error) { return "audit-token", nil },
		submit: func(_ context.Context, _ *apiclient.Client, auditLog types.LocalAgentAuditLogIngest) error {
			submitted = append(submitted, auditLog)
			return nil
		},
	}
	if err := onlineAudit.Run(auditTestCommand(string(payload), nil, nil), nil); err != nil {
		t.Fatal(err)
	}

	if len(submitted) != 2 {
		t.Fatalf("submitted events = %d, want current event plus one drained spooled event", len(submitted))
	}
	for i, event := range submitted {
		if event.EventID != expected.EventID {
			t.Fatalf("submitted[%d].EventID = %q, want %q", i, event.EventID, expected.EventID)
		}
	}
	if _, err := os.Stat(spool.path); !os.IsNotExist(err) {
		t.Fatalf("real spool file should be removed after successful drain, stat err = %v", err)
	}
}

func claudeCodeAuditPayload(t *testing.T, secretMarker, outputMarker string) []byte {
	t.Helper()

	payload := map[string]any{
		"session_id":      "spool-integration-session",
		"transcript_path": "/tmp/spool-integration-transcript.jsonl",
		"cwd":             "/tmp/spool-integration-workspace",
		"hook_event_name": "PostToolUse",
		"tool_name":       "Bash",
		"tool_input": map[string]string{
			"command": "echo " + secretMarker,
		},
		"tool_response": map[string]string{
			"status": "success",
			"output": outputMarker,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), secretMarker) || !strings.Contains(string(data), outputMarker) {
		t.Fatalf("test payload does not contain expected plaintext markers: %s", data)
	}
	return data
}
