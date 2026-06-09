package apiclient

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
)

func (c *Client) SubmitLocalAgentAuditLog(ctx context.Context, auditLog types.LocalAgentAuditLogIngest) (*types.LocalAgentAuditLogIngestResponse, error) {
	_, resp, err := c.postJSON(ctx, "/local-agent-audit-logs", auditLog)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.LocalAgentAuditLogIngestResponse{})
}
