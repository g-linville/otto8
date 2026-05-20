package setupstate

import (
	"errors"
	"testing"

	"github.com/obot-platform/obot/packaging/ObotSetup/internal/cli"
)

func TestBuildClassifiesMissingCLI(t *testing.T) {
	got := Build("/usr/local/bin/obot", "1.0.0", false, cli.Status{}, nil, nil, nil)
	if got.Kind != MissingCLI {
		t.Fatalf("kind = %s", got.Kind)
	}
}

func TestBuildClassifiesUnsupportedCLIOnStatusError(t *testing.T) {
	got := Build("/usr/local/bin/obot", "1.0.0", true, cli.Status{}, errors.New("unknown command"), nil, nil)
	if got.Kind != UnsupportedCLI {
		t.Fatalf("kind = %s", got.Kind)
	}
}

func TestBuildClassifiesUnsupportedCLIMissingCapabilities(t *testing.T) {
	status := cli.Status{
		Version:      "1.0.0",
		Capabilities: []string{"setup.nonInteractive"},
	}
	got := Build("/usr/local/bin/obot", "1.0.0", true, status, nil, nil, nil)
	if got.Kind != UnsupportedCLI {
		t.Fatalf("kind = %s", got.Kind)
	}
	if len(got.MissingCapabilities) != 2 {
		t.Fatalf("missing capabilities = %#v", got.MissingCapabilities)
	}
}

func TestBuildClassifiesFirstRun(t *testing.T) {
	got := Build("/usr/local/bin/obot", "1.0.0", true, validStatus("1.0.0", "", false, false), nil, nil, nil)
	if got.Kind != FirstRun {
		t.Fatalf("kind = %s", got.Kind)
	}
}

func TestBuildClassifiesNeedsLoginRepair(t *testing.T) {
	got := Build("/usr/local/bin/obot", "1.0.0", true, validStatus("1.0.0", "https://obot.example.com", false, false), nil, nil, nil)
	if got.Kind != NeedsLoginRepair {
		t.Fatalf("kind = %s", got.Kind)
	}
}

func TestBuildClassifiesAlreadyConfigured(t *testing.T) {
	got := Build("/usr/local/bin/obot", "1.0.0", true, validStatus("1.0.0", "https://obot.example.com", true, true), nil, nil, nil)
	if got.Kind != AlreadyConfigured {
		t.Fatalf("kind = %s", got.Kind)
	}
}

func TestBuildWarnsOnVersionMismatch(t *testing.T) {
	got := Build("/usr/local/bin/obot", "1.0.0", true, validStatus("1.1.0", "https://obot.example.com", true, true), nil, nil, nil)
	if !got.VersionMismatch {
		t.Fatal("expected version mismatch")
	}
}

func TestPathContains(t *testing.T) {
	if !PathContains("/bin:/usr/local/bin:/usr/bin", "/usr/local/bin") {
		t.Fatal("expected /usr/local/bin in path")
	}
	if PathContains("/bin:/usr/local/sbin:/usr/bin", "/usr/local/bin") {
		t.Fatal("did not expect /usr/local/bin in path")
	}
}

func validStatus(version, defaultURL string, tokenValid, setupComplete bool) cli.Status {
	return cli.Status{
		Version:       version,
		Capabilities:  RequiredCapabilities,
		DefaultURL:    defaultURL,
		TokenValid:    tokenValid,
		SetupComplete: setupComplete,
	}
}
