package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	obotcmd "github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/cli/internal/credentials"
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
		spool:      func(string) (auditSpool, error) { return &fakeAuditSpool{}, nil },
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
		spool:      func(string) (auditSpool, error) { return &fakeAuditSpool{}, nil },
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
		spool:      func(string) (auditSpool, error) { return &fakeAuditSpool{}, nil },
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
	spool := &fakeAuditSpool{}
	audit := Audit{
		Client:     "cursor",
		Event:      "post-tool-use-failure",
		root:       &Obot{Client: &apiclient.Client{BaseURL: "https://obot.example.com/api"}, Debug: true},
		auditToken: func(string) (string, error) { return "", context.Canceled },
		spool:      func(string) (auditSpool, error) { return spool, nil },
	}
	if err := audit.Run(auditTestCommand(string(fixture), nil, &stderr), nil); err != nil {
		t.Fatal(err)
	}
	if len(spool.appended) != 1 {
		t.Fatalf("appended events = %d, want 1", len(spool.appended))
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
		spool:      func(string) (auditSpool, error) { return &fakeAuditSpool{}, nil },
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

func TestAuditCommandFailedSubmissionAppendsAndDoesNotDrain(t *testing.T) {
	fixture, err := os.ReadFile("../localagents/testdata/audit-hooks/codex-cli/post-tool-use-failure.json")
	if err != nil {
		t.Fatal(err)
	}
	spool := &fakeAuditSpool{}
	audit := Audit{
		Client:     "codex-cli",
		Event:      "post-tool-use",
		root:       &Obot{Client: &apiclient.Client{BaseURL: "https://obot.example.com/api"}},
		auditToken: func(string) (string, error) { return "audit-token", nil },
		spool:      func(string) (auditSpool, error) { return spool, nil },
		submit: func(context.Context, *apiclient.Client, types.LocalAgentAuditLogIngest) error {
			return errors.New("offline")
		},
	}

	if err := audit.Run(auditTestCommand(string(fixture), nil, nil), nil); err != nil {
		t.Fatal(err)
	}
	if len(spool.appended) != 1 {
		t.Fatalf("appended events = %d, want 1", len(spool.appended))
	}
	if spool.drainCalls != 0 {
		t.Fatalf("drain calls = %d, want 0", spool.drainCalls)
	}
}

func TestAuditCommandSuccessfulSubmissionDrainsSpool(t *testing.T) {
	fixture, err := os.ReadFile("../localagents/testdata/audit-hooks/claude-code/post-tool-use-success.json")
	if err != nil {
		t.Fatal(err)
	}
	spool := &fakeAuditSpool{}
	audit := Audit{
		Client:     "claude-code",
		Event:      "post-tool-use",
		root:       &Obot{Client: &apiclient.Client{BaseURL: "https://obot.example.com/api"}},
		auditToken: func(string) (string, error) { return "audit-token", nil },
		spool:      func(string) (auditSpool, error) { return spool, nil },
		submit: func(context.Context, *apiclient.Client, types.LocalAgentAuditLogIngest) error {
			return nil
		},
	}

	if err := audit.Run(auditTestCommand(string(fixture), nil, nil), nil); err != nil {
		t.Fatal(err)
	}
	if len(spool.appended) != 0 {
		t.Fatalf("appended events = %d, want 0", len(spool.appended))
	}
	if spool.drainCalls != 1 {
		t.Fatalf("drain calls = %d, want 1", spool.drainCalls)
	}
}

func TestLocalAuditSpoolEncryptsDrainsAndClears(t *testing.T) {
	spool := newTestLocalAuditSpool(t)
	event := testAuditEvent("event-1", "secret-value")

	if err := spool.Append(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(spool.path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("secret-value")) || bytes.Contains(data, []byte("event-1")) {
		t.Fatalf("spool contains plaintext event data: %s", data)
	}

	var got []types.LocalAgentAuditLogIngest
	if err := spool.Drain(t.Context(), func(_ context.Context, auditLog types.LocalAgentAuditLogIngest) error {
		got = append(got, auditLog)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].EventID != "event-1" || string(got[0].RawToolInput) != `"secret-value"` {
		t.Fatalf("drained events = %#v", got)
	}
	if _, err := os.Stat(spool.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("spool file should be removed after full drain, stat err = %v", err)
	}
}

func TestLocalAuditSpoolFailedDrainLeavesFileIntact(t *testing.T) {
	spool := newTestLocalAuditSpool(t)
	if err := spool.Append(t.Context(), testAuditEvent("event-1", "secret-value")); err != nil {
		t.Fatal(err)
	}
	drainErr := errors.New("submit failed")

	err := spool.Drain(t.Context(), func(context.Context, types.LocalAgentAuditLogIngest) error {
		return drainErr
	})
	if !errors.Is(err, drainErr) {
		t.Fatalf("drain error = %v, want %v", err, drainErr)
	}
	if _, err := os.Stat(spool.path); err != nil {
		t.Fatalf("spool file should remain after failed drain: %v", err)
	}
}

func TestLocalAuditSpoolDropsOldestOnOverflow(t *testing.T) {
	spool := newTestLocalAuditSpool(t)
	if err := spool.Append(t.Context(), testAuditEvent("event-1", "first")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(spool.path)
	if err != nil {
		t.Fatal(err)
	}
	spool.maxSpoolBytes = info.Size() + 100
	if err := spool.Append(t.Context(), testAuditEvent("event-2", "two")); err != nil {
		t.Fatal(err)
	}

	var got []types.LocalAgentAuditLogIngest
	if err := spool.Drain(t.Context(), func(_ context.Context, auditLog types.LocalAgentAuditLogIngest) error {
		got = append(got, auditLog)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].EventID != "event-2" {
		t.Fatalf("drained events = %#v, want only newest event", got)
	}
}

func TestLocalAuditSpoolLockTimeout(t *testing.T) {
	spool := newTestLocalAuditSpool(t)
	spool.lockTimeout = 20 * time.Millisecond
	lock, err := acquireAuditSpoolLock(context.Background(), spool.path+".lock", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()

	err = spool.Append(context.Background(), testAuditEvent("event-1", "secret-value"))
	if !errors.Is(err, errAuditSpoolLockTimeout) {
		t.Fatalf("append error = %v, want lock timeout", err)
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

type fakeAuditSpool struct {
	appended   []types.LocalAgentAuditLogIngest
	drainCalls int
	appendErr  error
	drainErr   error
}

func (f *fakeAuditSpool) Append(_ context.Context, auditLog types.LocalAgentAuditLogIngest) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	f.appended = append(f.appended, auditLog)
	return nil
}

func (f *fakeAuditSpool) Drain(context.Context, auditSubmitFunc) error {
	f.drainCalls++
	return f.drainErr
}

type fakeCredentialStore struct {
	values map[string]string
}

func (f *fakeCredentialStore) Get(appURL string) (string, error) {
	if value, ok := f.values[appURL]; ok {
		return value, nil
	}
	return "", credentials.ErrNotFound
}

func (f *fakeCredentialStore) Set(appURL, token string) error {
	f.values[appURL] = token
	return nil
}

func (f *fakeCredentialStore) Delete(appURL string) error {
	delete(f.values, appURL)
	return nil
}

func newTestLocalAuditSpool(t *testing.T) *localAuditSpool {
	t.Helper()
	return &localAuditSpool{
		appURL:        "https://obot.example.com",
		path:          filepath.Join(t.TempDir(), "audit.spool"),
		keyStore:      &fakeCredentialStore{values: map[string]string{}},
		maxEventBytes: localagents.MaxAuditEventBytes,
		maxSpoolBytes: defaultAuditSpoolMaxBytes,
		lockTimeout:   defaultAuditSpoolLockTimeout,
	}
}

func testAuditEvent(eventID, input string) types.LocalAgentAuditLogIngest {
	return types.LocalAgentAuditLogIngest{
		LocalAgentAuditLogFields: types.LocalAgentAuditLogFields{
			EventID:      eventID,
			EventName:    "post-tool-use",
			ToolName:     "Bash",
			RawToolInput: json.RawMessage(strconvQuote(input)),
		},
		Client: types.LocalAgentAuditLogIngestClient{Name: "claude-code"},
	}
}

func strconvQuote(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
