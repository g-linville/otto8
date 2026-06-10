package apiclient

import (
	"context"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
)

func (c *Client) CreateAPIKey(ctx context.Context, req types.APIKeyCreateRequest) (*types.APIKeyCreateResponse, error) {
	_, resp, err := c.postJSON(ctx, "/api-keys", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.APIKeyCreateResponse{})
}

func (c *Client) InspectAPIKeySelf(ctx context.Context) (*types.APIKeySelfInspectionResponse, error) {
	_, resp, err := c.doRequest(ctx, http.MethodGet, "/api-keys-self", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.APIKeySelfInspectionResponse{})
}
