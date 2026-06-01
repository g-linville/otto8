package apiclient

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
)

func (c *Client) SubmitLocalAgentAuditLog(ctx context.Context, entry types.LocalAgentAuditLog) (*types.LocalAgentAuditLogResponse, error) {
	_, resp, err := c.postJSON(ctx, "/local-agent-audit-logs", entry)
	if err != nil {
		return nil, err
	}

	return toObject(resp, &types.LocalAgentAuditLogResponse{})
}
