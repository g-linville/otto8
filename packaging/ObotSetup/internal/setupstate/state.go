package setupstate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/obot-platform/obot/packaging/ObotSetup/internal/cli"
)

type Kind string

const (
	FirstRun          Kind = "first_run"
	AlreadyConfigured Kind = "already_configured"
	NeedsLoginRepair  Kind = "needs_login_repair"
	UnsupportedCLI    Kind = "unsupported_cli"
	MissingCLI        Kind = "missing_cli"
)

var RequiredCapabilities = []string{
	"setup.nonInteractive",
	"setup.detectAgents",
	"setup.progressJSON",
}

type Snapshot struct {
	Kind                Kind
	CLIPath             string
	CLIExists           bool
	Status              cli.Status
	StatusErr           error
	Agents              []cli.Agent
	AgentsErr           error
	AppVersion          string
	VersionMismatch     bool
	UsrLocalBinInPath   bool
	MissingCapabilities []string
}

func Build(cliPath, appVersion string, cliExists bool, status cli.Status, statusErr error, agents []cli.Agent, agentsErr error) Snapshot {
	s := Snapshot{
		CLIPath:           cliPath,
		CLIExists:         cliExists,
		Status:            status,
		StatusErr:         statusErr,
		Agents:            agents,
		AgentsErr:         agentsErr,
		AppVersion:        appVersion,
		UsrLocalBinInPath: PathContains(os.Getenv("PATH"), "/usr/local/bin"),
	}

	if !cliExists {
		s.Kind = MissingCLI
		return s
	}
	if statusErr != nil {
		s.Kind = UnsupportedCLI
		return s
	}

	s.MissingCapabilities = MissingCapabilities(status.Capabilities, RequiredCapabilities)
	if len(s.MissingCapabilities) > 0 {
		s.Kind = UnsupportedCLI
		return s
	}

	s.VersionMismatch = versionMismatch(appVersion, status.Version)
	switch {
	case status.SetupComplete:
		s.Kind = AlreadyConfigured
	case status.DefaultURL != "" && !status.TokenValid:
		s.Kind = NeedsLoginRepair
	default:
		s.Kind = FirstRun
	}
	return s
}

func MissingCapabilities(got, required []string) []string {
	present := map[string]bool{}
	for _, capability := range got {
		present[capability] = true
	}

	var missing []string
	for _, capability := range required {
		if !present[capability] {
			missing = append(missing, capability)
		}
	}
	return missing
}

func PathContains(pathValue, dir string) bool {
	return slices.Contains(filepath.SplitList(pathValue), dir)
}

func versionMismatch(appVersion, cliVersion string) bool {
	appVersion = strings.TrimSpace(appVersion)
	cliVersion = strings.TrimSpace(cliVersion)
	if appVersion == "" || appVersion == "dev" || cliVersion == "" {
		return false
	}
	return appVersion != cliVersion
}
