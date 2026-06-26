package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

func TestInsertMCPAuditLogsAllowsMultipleMCPRowsWithLocalAgentIndexes(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	logs := []types.MCPAuditLog{
		{
			CreatedAt: now,
			UserID:    "user-1",
			ClientIP:  "127.0.0.1",
			MCPFields: &types.MCPAuditLogFields{
				MCPID:          "mcp-1",
				CallType:       "tools/call",
				CallIdentifier: "tool-1",
				RequestBody:    json.RawMessage(`{"name":"tool-1"}`),
			},
		},
		{
			CreatedAt: now.Add(time.Second),
			UserID:    "user-2",
			ClientIP:  "127.0.0.2",
			MCPFields: &types.MCPAuditLogFields{
				MCPID:          "mcp-2",
				CallType:       "tools/call",
				CallIdentifier: "tool-2",
				RequestBody:    json.RawMessage(`{"name":"tool-2"}`),
			},
		},
	}

	if err := c.insertAuditLogs(ctx, logs); err != nil {
		t.Fatalf("insert MCP audit logs: %v", err)
	}

	if got := countAuditLogs(t, c); got != 2 {
		t.Fatalf("expected 2 audit logs, got %d", got)
	}
}

func TestInsertMCPAuditLogsMergesResponseOnlyRowWithGroupedFields(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	now := time.Now().UTC()

	request := types.MCPAuditLog{
		CreatedAt: now,
		UserID:    "user-1",
		MCPFields: &types.MCPAuditLogFields{
			MCPID:       "mcp-1",
			RequestID:   "request-1",
			SessionID:   "session-1",
			RequestBody: json.RawMessage(`{"name":"tool"}`),
		},
	}
	response := types.MCPAuditLog{
		CreatedAt: now.Add(250 * time.Millisecond),
		UserID:    "user-1",
		MCPFields: &types.MCPAuditLogFields{
			MCPID:            "mcp-1",
			RequestID:        "request-1",
			SessionID:        "session-1",
			ResponseReceived: true,
			ResponseBody:     json.RawMessage(`{"ok":true}`),
			ResponseStatus:   200,
		},
	}

	if err := c.insertAuditLogs(ctx, []types.MCPAuditLog{request}); err != nil {
		t.Fatalf("insert request audit log: %v", err)
	}
	if err := c.insertAuditLogs(ctx, []types.MCPAuditLog{response}); err != nil {
		t.Fatalf("insert response audit log: %v", err)
	}

	var got types.MCPAuditLog
	if err := c.db.WithContext(ctx).First(&got).Error; err != nil {
		t.Fatalf("load merged audit log: %v", err)
	}
	if got.MCP().ResponseReceived != true {
		t.Fatal("expected response_received to be true")
	}
	if string(got.MCP().ResponseBody) != `{"ok":true}` {
		t.Fatalf("expected response body to be merged, got %s", got.MCP().ResponseBody)
	}
	if got.MCP().ProcessingTimeMs != 250 {
		t.Fatalf("expected processing time 250ms, got %d", got.MCP().ProcessingTimeMs)
	}
}

func TestCreateLocalAgentToolCallAuditLogsInsertsCompletedEntries(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	for _, status := range []types2.LocalAgentAuditLogStatus{
		types2.LocalAgentAuditLogStatusSucceeded,
		types2.LocalAgentAuditLogStatusFailed,
		types2.LocalAgentAuditLogStatusDenied,
		types2.LocalAgentAuditLogStatusTimeout,
	} {
		entry := testLocalAgentAuditLog(fmt.Sprintf("entry-%s", status), status, now)
		if err := c.CreateLocalAgentToolCallAuditLogs(ctx, []types.MCPAuditLog{entry}); err != nil {
			t.Fatalf("create local agent audit log for status %q: %v", status, err)
		}
	}

	if got := countAuditLogs(t, c); got != 4 {
		t.Fatalf("expected 4 audit logs, got %d", got)
	}

	var got types.MCPAuditLog
	if err := c.db.WithContext(ctx).Where("idempotency_key = ?", "entry-succeeded").First(&got).Error; err != nil {
		t.Fatalf("load local agent audit log: %v", err)
	}
	if got.SourceType != types2.AuditLogSourceTypeLocalAgentToolCall {
		t.Fatalf("expected local-agent source type, got %q", got.SourceType)
	}
	if got.LocalAgentToolCallFields == nil || got.LocalAgentToolCallFields.ToolName != "mcp__server__tool" {
		t.Fatalf("local-agent fields were not persisted: %#v", got.LocalAgentToolCallFields)
	}
}

func TestCreateLocalAgentToolCallAuditLogsDuplicateIdempotencyKeyIsNoop(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	entry := testLocalAgentAuditLog("entry-1", types2.LocalAgentAuditLogStatusSucceeded, now)
	if err := c.CreateLocalAgentToolCallAuditLogs(ctx, []types.MCPAuditLog{entry}); err != nil {
		t.Fatalf("create local agent audit log: %v", err)
	}
	entry.LocalAgentToolCallFields.ToolName = "different-tool"
	if err := c.CreateLocalAgentToolCallAuditLogs(ctx, []types.MCPAuditLog{entry}); err != nil {
		t.Fatalf("duplicate local agent audit log should be a no-op: %v", err)
	}

	if got := countAuditLogs(t, c); got != 1 {
		t.Fatalf("expected duplicate idempotency key to leave 1 audit log, got %d", got)
	}

	var got types.MCPAuditLog
	if err := c.db.WithContext(ctx).Where("idempotency_key = ?", "entry-1").First(&got).Error; err != nil {
		t.Fatalf("load local agent audit log: %v", err)
	}
	if got.LocalAgentToolCallFields.ToolName != "mcp__server__tool" {
		t.Fatalf("expected original row to be preserved, got tool name %q", got.LocalAgentToolCallFields.ToolName)
	}
}

func TestCreateLocalAgentToolCallAuditLogsRejectsMissingRequiredFields(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	tests := map[string]func(*types.LocalAgentToolCallAuditLogFields){
		"agent provider":   func(f *types.LocalAgentToolCallAuditLogFields) { f.AgentProvider = "" },
		"observed at":      func(f *types.LocalAgentToolCallAuditLogFields) { f.ObservedAt = time.Time{} },
		"tool name":        func(f *types.LocalAgentToolCallAuditLogFields) { f.ToolName = "" },
		"tool input":       func(f *types.LocalAgentToolCallAuditLogFields) { f.ToolInput = nil },
		"status":           func(f *types.LocalAgentToolCallAuditLogFields) { f.Status = "" },
		"idempotency key":  func(f *types.LocalAgentToolCallAuditLogFields) { f.IdempotencyKey = "" },
		"raw hook payload": func(f *types.LocalAgentToolCallAuditLogFields) { f.RawHookPayload = nil },
		"CLI version":      func(f *types.LocalAgentToolCallAuditLogFields) { f.CLIVersion = "" },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			entry := testLocalAgentAuditLog("entry-"+name, types2.LocalAgentAuditLogStatusSucceeded, now)
			mutate(entry.LocalAgentToolCallFields)
			if err := c.CreateLocalAgentToolCallAuditLogs(ctx, []types.MCPAuditLog{entry}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCreateLocalAgentToolCallAuditLogsRejectsNonTerminalStatus(t *testing.T) {
	c := newTestClient(t)
	entry := testLocalAgentAuditLog("entry-1", types2.LocalAgentAuditLogStatusSucceeded, time.Now().UTC())
	entry.LocalAgentToolCallFields.Status = string(types2.LocalAgentAuditLogPhasePreTool)

	if err := c.CreateLocalAgentToolCallAuditLogs(t.Context(), []types.MCPAuditLog{entry}); err == nil {
		t.Fatal("expected pre-tool status to be rejected")
	}
}

func TestLocalAgentAuditLogEncryptedFieldsDecryptOnlyWhenRequested(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			mcpAuditLogGroupResource: testAuditLogTransformer{},
		},
	}
	ctx := t.Context()

	entry := testLocalAgentAuditLog("entry-1", types2.LocalAgentAuditLogStatusSucceeded, time.Now().UTC())
	entry.LocalAgentToolCallFields.ToolOutput = json.RawMessage(`{"ok":true}`)
	entry.LocalAgentToolCallFields.TranscriptPath = "/Users/test/.agent/transcript.jsonl"
	if err := c.CreateLocalAgentToolCallAuditLogs(ctx, []types.MCPAuditLog{entry}); err != nil {
		t.Fatalf("create local agent audit log: %v", err)
	}

	var stored types.MCPAuditLog
	if err := c.db.WithContext(ctx).Where("idempotency_key = ?", "entry-1").First(&stored).Error; err != nil {
		t.Fatalf("load stored local agent audit log: %v", err)
	}
	if !stored.Encrypted {
		t.Fatal("expected stored local-agent audit log to be marked encrypted")
	}
	local := stored.LocalAgentToolCallFields
	if bytes.Equal(local.ToolInput, entry.LocalAgentToolCallFields.ToolInput) ||
		bytes.Equal(local.ToolOutput, entry.LocalAgentToolCallFields.ToolOutput) ||
		bytes.Equal(local.RawHookPayload, entry.LocalAgentToolCallFields.RawHookPayload) ||
		local.TranscriptPath == entry.LocalAgentToolCallFields.TranscriptPath {
		t.Fatalf("expected sensitive local-agent fields to be encrypted before storage: %#v", local)
	}

	withoutPayloads, err := c.GetMCPAuditLog(ctx, stored.ID, false)
	if err != nil {
		t.Fatalf("get local agent audit log without payloads: %v", err)
	}
	local = withoutPayloads.LocalAgentToolCallFields
	if len(local.ToolInput) != 0 || len(local.ToolOutput) != 0 || len(local.RawHookPayload) != 0 || local.TranscriptPath != "" {
		t.Fatalf("expected sensitive fields to be scrubbed without payload access: %#v", local)
	}

	withPayloads, err := c.GetMCPAuditLog(ctx, stored.ID, true)
	if err != nil {
		t.Fatalf("get local agent audit log with payloads: %v", err)
	}
	local = withPayloads.LocalAgentToolCallFields
	if string(local.ToolInput) != `{"arg":true}` ||
		string(local.ToolOutput) != `{"ok":true}` ||
		string(local.RawHookPayload) != `{"native":true}` ||
		local.TranscriptPath != "/Users/test/.agent/transcript.jsonl" {
		t.Fatalf("expected sensitive local-agent fields to decrypt, got %#v", local)
	}
}

func testLocalAgentAuditLog(idempotencyKey string, status types2.LocalAgentAuditLogStatus, observedAt time.Time) types.MCPAuditLog {
	return types.MCPAuditLog{
		SourceType: types2.AuditLogSourceTypeLocalAgentToolCall,
		UserID:     "user-1",
		ClientIP:   "127.0.0.1",
		LocalAgentToolCallFields: &types.LocalAgentToolCallAuditLogFields{
			AgentProvider:          string(types2.LocalAgentProviderCodex),
			CLIVersion:             "1.0.0",
			Status:                 string(status),
			ObservedAt:             observedAt,
			IdempotencyKey:         idempotencyKey,
			ToolUseID:              "tool-use-1",
			SessionID:              "session-1",
			ToolName:               "mcp__server__tool",
			ToolKind:               "mcp",
			ObotAuditCorrelationID: "correlation-1",
			IdentityStatus:         string(types2.LocalAgentIdentityStatusAuthenticatedUser),
			ToolInput:              json.RawMessage(`{"arg":true}`),
			RawHookPayload:         json.RawMessage(`{"native":true}`),
		},
	}
}

type testAuditLogTransformer struct{}

func (testAuditLogTransformer) TransformToStorage(_ context.Context, data []byte, _ value.Context) ([]byte, error) {
	return append([]byte("encrypted:"), data...), nil
}

func (testAuditLogTransformer) TransformFromStorage(_ context.Context, data []byte, _ value.Context) ([]byte, bool, error) {
	out, ok := bytes.CutPrefix(data, []byte("encrypted:"))
	if !ok {
		return nil, false, errors.New("missing encrypted prefix")
	}
	return out, false, nil
}
