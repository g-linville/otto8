package types

// APIKey mirrors pkg/gateway/types.APIKey for the public API-key HTTP
// contract exposed through the apiclient module.
type APIKey struct {
	ID                 uint     `json:"id"`
	UserID             uint     `json:"userId"`
	Name               string   `json:"name"`
	Description        string   `json:"description,omitempty"`
	CanAccessSkills    bool     `json:"canAccessSkills"`
	CanAppendAuditLogs bool     `json:"canAppendAuditLogs"`
	CreatedAt          Time     `json:"createdAt"`
	LastUsedAt         *Time    `json:"lastUsedAt,omitempty"`
	ExpiresAt          *Time    `json:"expiresAt,omitempty"`
	MCPServerIDs       []string `json:"mcpServerIds,omitempty"`
}

// APIKeyCreateRequest mirrors the gateway API-key create request used by
// pkg/gateway/server.createAPIKey.
type APIKeyCreateRequest struct {
	Name               string   `json:"name"`
	Description        string   `json:"description,omitempty"`
	ExpiresAt          *Time    `json:"expiresAt,omitempty"`
	MCPServerIDs       []string `json:"mcpServerIds,omitempty"`
	CanAccessSkills    bool     `json:"canAccessSkills"`
	CanAppendAuditLogs bool     `json:"canAppendAuditLogs"`
}

// APIKeyCreateResponse mirrors pkg/gateway/types.APIKeyCreateResponse for
// the API-key creation HTTP response.
type APIKeyCreateResponse struct {
	APIKey
	Key string `json:"key"`
}

// APIKeySelfInspectionIdentity mirrors
// pkg/gateway/types.APIKeySelfInspectionIdentity.
type APIKeySelfInspectionIdentity struct {
	Subject     string `json:"subject,omitempty"`
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
}

// APIKeySelfInspectionResponse mirrors
// pkg/gateway/types.APIKeySelfInspectionResponse.
type APIKeySelfInspectionResponse struct {
	APIKey
	Identity APIKeySelfInspectionIdentity `json:"identity"`
}
