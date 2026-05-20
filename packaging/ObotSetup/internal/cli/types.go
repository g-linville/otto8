package cli

type Status struct {
	Version       string   `json:"version"`
	Capabilities  []string `json:"capabilities"`
	DefaultURL    string   `json:"defaultURL"`
	TokenValid    bool     `json:"tokenValid"`
	SetupComplete bool     `json:"setupComplete"`
}

type DetectAgentsResult struct {
	Agents []Agent `json:"agents"`
}

type Agent struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	State       string `json:"state"`
	Reason      string `json:"reason"`
}
