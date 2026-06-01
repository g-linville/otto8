package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestLocalAgentAuditSubmitAccepted(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/local-agent-audit-logs", strings.NewReader(`{
  "source": "claude-code",
  "eventName": "PostToolUse",
  "toolName": "Bash",
  "sessionID": "session-1",
  "toolUseID": "toolu_1",
  "success": true,
  "toolInput": {"command": "pwd"},
  "toolResponse": {"stdout": "/repo\n"}
}`))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	err := NewLocalAgentAuditHandler().Submit(api.Context{
		ResponseWriter: rec,
		Request:        req,
		GatewayClient:  &gclient.Client{},
		User:           &user.DefaultInfo{UID: "user-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"accepted":true`) {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestLocalAgentAuditSubmitRejectsUnsupportedSource(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/local-agent-audit-logs", strings.NewReader(`{
  "source": "other",
  "toolName": "Bash"
}`))
	rec := httptest.NewRecorder()

	err := NewLocalAgentAuditHandler().Submit(api.Context{
		ResponseWriter: rec,
		Request:        req,
		GatewayClient:  &gclient.Client{},
		User:           &user.DefaultInfo{UID: "user-1"},
	})
	httpErr, ok := err.(*types.ErrHTTP)
	if !ok {
		t.Fatalf("error = %T %v, want ErrHTTP", err, err)
	}
	if httpErr.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want %d", httpErr.Code, http.StatusBadRequest)
	}
}

func TestRemoteIP(t *testing.T) {
	if got := remoteIP("192.0.2.1:1234"); got != "192.0.2.1" {
		t.Fatalf("remoteIP = %q", got)
	}
	if got := remoteIP("not-host-port"); got != "not-host-port" {
		t.Fatalf("remoteIP fallback = %q", got)
	}
}
