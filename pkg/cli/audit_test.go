package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	obotcmd "github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/localagents"
	"github.com/spf13/cobra"
)

func TestAuditCommandRejectsMissingFlags(t *testing.T) {
	for _, tt := range []struct {
		name  string
		audit Audit
		want  string
	}{
		{name: "missing client", audit: Audit{Event: "post-tool-use"}, want: "--client is required"},
		{name: "missing event", audit: Audit{Client: "claude-code"}, want: "--event is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.audit.Run(auditTestCommand("", nil, nil), nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestAuditCommandSubmitsNormalizedEvent(t *testing.T) {
	fixture, err := os.ReadFile("../localagents/testdata/audit-hooks/claude-code/post-tool-use-success.json")
	if err != nil {
		t.Fatal(err)
	}

	var got types.LocalAgentAuditLogIngest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/local-agent-audit-logs" {
			t.Fatalf("path = %s, want /local-agent-audit-logs", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer audit-token" {
			t.Fatalf("authorization = %q, want audit token", auth)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(types.LocalAgentAuditLogIngestResponse{Accepted: 1, Inserted: 1})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	audit := Audit{
		Client:     "claude-code",
		Event:      "post-tool-use",
		root:       &Obot{Client: &apiclient.Client{BaseURL: server.URL}},
		auditToken: func(string) (string, error) { return "audit-token", nil },
	}
	if err := audit.Run(auditTestCommand(string(fixture), &stdout, &stderr), nil); err != nil {
		t.Fatal(err)
	}

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if got.Client.Name != "claude-code" || got.EventName != "PostToolUse" || got.ToolName != "Bash" {
		t.Fatalf("unexpected normalized event: %#v", got)
	}
	if got.Success == nil || !*got.Success {
		t.Fatalf("success = %v, want true", got.Success)
	}
	if got.EventID == "" || got.RawToolInput == nil || got.RawToolOutput == nil {
		t.Fatalf("expected event ID and raw payloads: %#v", got)
	}
}

func TestAuditCommandGeneratedFlags(t *testing.T) {
	fixture, err := os.ReadFile("../localagents/testdata/audit-hooks/claude-code/post-tool-use-success.json")
	if err != nil {
		t.Fatal(err)
	}

	var got types.LocalAgentAuditLogIngest
	audit := &Audit{
		root:       &Obot{Client: &apiclient.Client{BaseURL: "https://obot.example.com/api"}},
		auditToken: func(string) (string, error) { return "audit-token", nil },
		submit: func(_ context.Context, _ *apiclient.Client, auditLog types.LocalAgentAuditLogIngest) error {
			got = auditLog
			return nil
		},
	}
	cmd := obotcmd.Command(audit)
	if !cmd.Hidden {
		t.Fatalf("audit command should be hidden")
	}
	cmd.SetContext(context.Background())
	cmd.SetIn(bytes.NewReader(fixture))
	cmd.SetArgs([]string{"--client", "claude-code", "--event", "post-tool-use"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Client.Name != "claude-code" || got.EventName != "PostToolUse" {
		t.Fatalf("generated flags did not populate audit command: %#v", got)
	}
}

func TestAuditCommandDeliveryFailureIsQuietAndReturnsZero(t *testing.T) {
	fixture, err := os.ReadFile("../localagents/testdata/audit-hooks/codex-cli/post-tool-use-failure.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	audit := Audit{
		Client:     "codex-cli",
		Event:      "post-tool-use",
		root:       &Obot{Client: &apiclient.Client{BaseURL: server.URL}},
		auditToken: func(string) (string, error) { return "audit-token", nil },
	}
	if err := audit.Run(auditTestCommand(string(fixture), &stdout, &stderr), nil); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected quiet failure, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestAuditCommandDebugPrintsDiagnostics(t *testing.T) {
	fixture, err := os.ReadFile("../localagents/testdata/audit-hooks/cursor/post-tool-use-failure.json")
	if err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	audit := Audit{
		Client:     "cursor",
		Event:      "post-tool-use-failure",
		root:       &Obot{Client: &apiclient.Client{BaseURL: "https://obot.example.com/api"}, Debug: true},
		auditToken: func(string) (string, error) { return "", context.Canceled },
	}
	if err := audit.Run(auditTestCommand(string(fixture), nil, &stderr), nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "submit audit event") {
		t.Fatalf("stderr = %q, want debug diagnostic", stderr.String())
	}
	if strings.Contains(stderr.String(), "audit-token") {
		t.Fatalf("debug output should not include secrets: %q", stderr.String())
	}
}

func TestAuditCommandOversizedInputSubmitsMetadata(t *testing.T) {
	var got types.LocalAgentAuditLogIngest
	audit := Audit{
		Client:     "cursor",
		Event:      "after-shell-execution",
		root:       &Obot{Client: &apiclient.Client{BaseURL: "https://obot.example.com/api"}},
		auditToken: func(string) (string, error) { return "audit-token", nil },
		submit: func(_ context.Context, _ *apiclient.Client, auditLog types.LocalAgentAuditLogIngest) error {
			got = auditLog
			return nil
		},
	}

	input := strings.Repeat("{", localagents.MaxAuditEventBytes+100)
	if err := audit.Run(auditTestCommand(input, nil, nil), nil); err != nil {
		t.Fatal(err)
	}
	if !got.PayloadTruncated {
		t.Fatalf("payloadTruncated = false, want true")
	}
	if got.Client.Name != "cursor" || got.EventName != "after-shell-execution" || got.EventID == "" {
		t.Fatalf("metadata was not preserved: %#v", got)
	}
	if len(got.RawClientHookEvent) != 0 || got.RawToolInput != nil || got.RawToolOutput != nil {
		t.Fatalf("raw payloads should be dropped for oversized malformed input: %#v", got)
	}
}

func auditTestCommand(stdin string, stdout, stderr *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetIn(strings.NewReader(stdin))
	if stdout != nil {
		cmd.SetOut(stdout)
	}
	if stderr != nil {
		cmd.SetErr(stderr)
	}
	return cmd
}
