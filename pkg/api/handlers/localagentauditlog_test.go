package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestLocalAgentAuditLogSubmitCreatesAuthenticatedRows(t *testing.T) {
	client := newTestGatewayClient(t)
	log := testLocalAgentAuditLog("entry-1", apitypes.LocalAgentAuditLogStatusSucceeded)
	log.UserID = "spoofed-user"
	log.ClientIP = "spoofed-ip"
	log.LocalAgentToolCallFields.IdentityStatus = string(apitypes.LocalAgentIdentityStatusAnonymousDevice)

	body, err := json.Marshal([]gatewaytypes.MCPAuditLog{log})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/local-agent-audit-logs", bytes.NewReader(body))
	req.RemoteAddr = "203.0.113.1:12345"
	rec := httptest.NewRecorder()

	err = NewLocalAgentAuditLogHandler().Submit(api.Context{
		ResponseWriter: rec,
		Request:        req,
		GatewayClient:  client,
		User: &user.DefaultInfo{
			UID: "user-1",
		},
	})
	if err != nil {
		t.Fatalf("submit local-agent audit log: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}

	stored, err := client.GetMCPAuditLog(t.Context(), 1, false)
	if err != nil {
		t.Fatalf("load stored audit log: %v", err)
	}
	if stored.UserID != "user-1" || stored.ClientIP != "203.0.113.1:12345" {
		t.Fatalf("expected server attribution, got userID=%q clientIP=%q", stored.UserID, stored.ClientIP)
	}
	if stored.LocalAgentToolCallFields.IdentityStatus != string(apitypes.LocalAgentIdentityStatusAuthenticatedUser) {
		t.Fatalf("expected authenticated identity status, got %q", stored.LocalAgentToolCallFields.IdentityStatus)
	}
}

func TestPrepareLocalAgentAuditLogBatchAttributesAuthenticatedUser(t *testing.T) {
	log := testLocalAgentAuditLog("entry-1", apitypes.LocalAgentAuditLogStatusSucceeded)
	log.ID = 99
	log.UserID = "spoofed-user"
	log.ClientIP = "spoofed-ip"
	log.Encrypted = true
	log.LocalAgentToolCallFields.IdentityStatus = string(apitypes.LocalAgentIdentityStatusAnonymousDevice)

	prepared, err := prepareLocalAgentAuditLogBatch([]gatewaytypes.MCPAuditLog{log}, "user-1", "203.0.113.1")
	if err != nil {
		t.Fatalf("prepare batch: %v", err)
	}
	if len(prepared) != 1 {
		t.Fatalf("expected one prepared log, got %d", len(prepared))
	}

	got := prepared[0]
	if got.ID != 0 {
		t.Fatalf("expected server to clear client-provided ID, got %d", got.ID)
	}
	if got.UserID != "user-1" || got.ClientIP != "203.0.113.1" {
		t.Fatalf("expected server attribution, got userID=%q clientIP=%q", got.UserID, got.ClientIP)
	}
	if got.Encrypted {
		t.Fatal("expected server to clear client-provided encrypted flag")
	}
	if got.LocalAgentToolCallFields.IdentityStatus != string(apitypes.LocalAgentIdentityStatusAuthenticatedUser) {
		t.Fatalf("expected authenticated identity status, got %q", got.LocalAgentToolCallFields.IdentityStatus)
	}
}

func newTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()

	services, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
	if err != nil {
		t.Fatalf("create storage services: %v", err)
	}

	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("auto-migrate gateway db: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	client := gatewayclient.New(ctx, db, nil, nil, nil, nil, time.Hour, 100, 0)
	t.Cleanup(func() {
		cancel()
		_ = client.Close()
	})
	return client
}

func TestPrepareLocalAgentAuditLogBatchRejectsInvalidEntries(t *testing.T) {
	tests := map[string]func(gatewaytypes.MCPAuditLog) gatewaytypes.MCPAuditLog{
		"empty batch": nil,
		"missing source type": func(log gatewaytypes.MCPAuditLog) gatewaytypes.MCPAuditLog {
			log.SourceType = ""
			return log
		},
		"mcp source type": func(log gatewaytypes.MCPAuditLog) gatewaytypes.MCPAuditLog {
			log.SourceType = apitypes.AuditLogSourceTypeMCP
			return log
		},
		"mcp fields populated": func(log gatewaytypes.MCPAuditLog) gatewaytypes.MCPAuditLog {
			log.MCPFields = &gatewaytypes.MCPAuditLogFields{MCPID: "mcp-1"}
			return log
		},
		"missing idempotency key": func(log gatewaytypes.MCPAuditLog) gatewaytypes.MCPAuditLog {
			log.LocalAgentToolCallFields.IdempotencyKey = ""
			return log
		},
		"non terminal status": func(log gatewaytypes.MCPAuditLog) gatewaytypes.MCPAuditLog {
			log.LocalAgentToolCallFields.Status = "started"
			return log
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var logs []gatewaytypes.MCPAuditLog
			if mutate != nil {
				logs = []gatewaytypes.MCPAuditLog{mutate(testLocalAgentAuditLog("entry-1", apitypes.LocalAgentAuditLogStatusSucceeded))}
			}

			_, err := prepareLocalAgentAuditLogBatch(logs, "user-1", "203.0.113.1")
			var errHTTP *apitypes.ErrHTTP
			if !errors.As(err, &errHTTP) {
				t.Fatalf("expected ErrHTTP, got %T: %v", err, err)
			}
			if errHTTP.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", errHTTP.Code)
			}
		})
	}
}

func testLocalAgentAuditLog(idempotencyKey string, status apitypes.LocalAgentAuditLogStatus) gatewaytypes.MCPAuditLog {
	observedAt := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	return gatewaytypes.MCPAuditLog{
		CreatedAt:  observedAt,
		SourceType: apitypes.AuditLogSourceTypeLocalAgentToolCall,
		LocalAgentToolCallFields: &gatewaytypes.LocalAgentToolCallAuditLogFields{
			AgentProvider:  string(apitypes.LocalAgentProviderCodex),
			CLIVersion:     "1.0.0",
			Status:         string(status),
			ObservedAt:     observedAt,
			IdempotencyKey: idempotencyKey,
			ToolName:       "mcp__server__tool",
			ToolInput:      json.RawMessage(`{"arg":true}`),
			RawHookPayload: json.RawMessage(`{"native":true}`),
		},
	}
}
