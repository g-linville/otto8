package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	localAgentAuditMCPID       = "local-agent:claude-code"
	localAgentAuditDisplayName = "Claude Code Local Tools"
)

type LocalAgentAuditHandler struct{}

func NewLocalAgentAuditHandler() *LocalAgentAuditHandler {
	return &LocalAgentAuditHandler{}
}

func (*LocalAgentAuditHandler) Submit(req api.Context) error {
	var input types.LocalAgentAuditLog
	if err := req.Read(&input); err != nil {
		return err
	}
	if input.Source != "claude-code" {
		return types.NewErrBadRequest("unsupported local agent source %q", input.Source)
	}
	if input.ToolName == "" {
		return types.NewErrBadRequest("toolName is required")
	}

	createdAt := input.CreatedAt.GetTime()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	} else {
		createdAt = createdAt.UTC()
	}

	status := http.StatusOK
	if !input.Success {
		status = http.StatusInternalServerError
	}

	requestBody, err := marshalLocalAgentRequestBody(input)
	if err != nil {
		return err
	}

	auditLog := gatewaytypes.MCPAuditLog{
		CreatedAt:                 createdAt,
		UserID:                    req.User.GetUID(),
		MCPID:                     localAgentAuditMCPID,
		MCPServerDisplayName:      localAgentAuditDisplayName,
		MCPServerCatalogEntryName: "local-agent",
		ClientName:                "claude-code",
		ClientIP:                  remoteIP(req.Request.RemoteAddr),
		CallType:                  "tool",
		CallIdentifier:            input.ToolName,
		RequestBody:               requestBody,
		ResponseBody:              input.ToolResponse,
		ResponseStatus:            status,
		Error:                     input.Error,
		ProcessingTimeMs:          input.DurationMs,
		SessionID:                 input.SessionID,
		RequestID:                 input.ToolUseID,
		UserAgent:                 req.Request.UserAgent(),
		ResponseReceived:          true,
	}
	req.GatewayClient.LogMCPAuditEntry(auditLog)

	return req.WriteCreated(types.LocalAgentAuditLogResponse{Accepted: true})
}

func marshalLocalAgentRequestBody(input types.LocalAgentAuditLog) (json.RawMessage, error) {
	body := struct {
		EventName      string          `json:"eventName"`
		ToolInput      json.RawMessage `json:"toolInput,omitempty"`
		CWD            string          `json:"cwd,omitempty"`
		TranscriptPath string          `json:"transcriptPath,omitempty"`
		Raw            json.RawMessage `json:"raw,omitempty"`
	}{
		EventName:      input.EventName,
		ToolInput:      input.ToolInput,
		CWD:            input.CWD,
		TranscriptPath: input.TranscriptPath,
		Raw:            input.Raw,
	}
	data, err := json.Marshal(body)
	return json.RawMessage(data), err
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
