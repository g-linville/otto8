package handlers

import (
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

type LocalAgentAuditLogHandler struct{}

func NewLocalAgentAuditLogHandler() *LocalAgentAuditLogHandler {
	return &LocalAgentAuditLogHandler{}
}

type localAgentAuditLogSubmitResponse struct {
	Accepted int `json:"accepted"`
}

// Submit handles POST /api/local-agent-audit-logs.
func (*LocalAgentAuditLogHandler) Submit(req api.Context) error {
	var logs []gatewaytypes.MCPAuditLog
	if err := req.Read(&logs); err != nil {
		return types.NewErrBadRequest("failed to read input: %v", err)
	}

	logs, err := prepareLocalAgentAuditLogBatch(logs, req.User.GetUID(), requestinfo.GetSourceIP(req.Request))
	if err != nil {
		return err
	}

	if err := req.GatewayClient.CreateLocalAgentToolCallAuditLogs(req.Context(), logs); err != nil {
		return err
	}

	return req.WriteCode(localAgentAuditLogSubmitResponse{Accepted: len(logs)}, http.StatusAccepted)
}

func prepareLocalAgentAuditLogBatch(logs []gatewaytypes.MCPAuditLog, userID, clientIP string) ([]gatewaytypes.MCPAuditLog, error) {
	if len(logs) == 0 {
		return nil, types.NewErrBadRequest("at least one audit log is required")
	}

	out := make([]gatewaytypes.MCPAuditLog, len(logs))
	for i, log := range logs {
		if log.SourceType != types.AuditLogSourceTypeLocalAgentToolCall {
			return nil, types.NewErrBadRequest("audit log %d sourceType must be %q", i, types.AuditLogSourceTypeLocalAgentToolCall)
		}
		if err := log.ValidateSourceFields(); err != nil {
			return nil, types.NewErrBadRequest("invalid audit log %d source fields: %v", i, err)
		}

		local := log.LocalAgentToolCall()
		local.IdentityStatus = string(types.LocalAgentIdentityStatusAuthenticatedUser)

		log.ID = 0 // this will be automatically set by GORM
		log.UserID = userID
		log.ClientIP = clientIP
		log.Encrypted = false

		if err := log.ValidateCompletedLocalAgentToolCall(); err != nil {
			return nil, types.NewErrBadRequest("invalid audit log %d: %v", i, err)
		}
		out[i] = log
	}

	return out, nil
}
