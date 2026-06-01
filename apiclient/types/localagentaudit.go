package types

import "encoding/json"

type LocalAgentAuditLog struct {
	Source         string          `json:"source"`
	EventName      string          `json:"eventName"`
	ToolName       string          `json:"toolName"`
	SessionID      string          `json:"sessionID,omitempty"`
	ToolUseID      string          `json:"toolUseID,omitempty"`
	CWD            string          `json:"cwd,omitempty"`
	TranscriptPath string          `json:"transcriptPath,omitempty"`
	DurationMs     int64           `json:"durationMs,omitempty"`
	Success        bool            `json:"success"`
	Error          string          `json:"error,omitempty"`
	ToolInput      json.RawMessage `json:"toolInput,omitempty"`
	ToolResponse   json.RawMessage `json:"toolResponse,omitempty"`
	Raw            json.RawMessage `json:"raw,omitempty"`
	CreatedAt      Time            `json:"createdAt,omitzero"`
}

type LocalAgentAuditLogResponse struct {
	Accepted bool `json:"accepted"`
}
