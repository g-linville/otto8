package cli

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeRunner struct {
	wantPath string
	wantArgs []string
	out      []byte
	err      error
}

func (f fakeRunner) Run(_ context.Context, path string, args ...string) ([]byte, error) {
	if path != f.wantPath {
		return nil, errors.New("unexpected path: " + path)
	}
	if !reflect.DeepEqual(args, f.wantArgs) {
		return nil, errors.New("unexpected args")
	}
	return f.out, f.err
}

func TestStatusParsesCLIJSON(t *testing.T) {
	client := Client{
		Path: "/tmp/obot",
		Runner: fakeRunner{
			wantPath: "/tmp/obot",
			wantArgs: []string{"setup", "status", "--json"},
			out: []byte(`{
				"version": "1.2.3",
				"capabilities": ["setup.nonInteractive", "setup.detectAgents", "setup.progressJSON"],
				"defaultURL": "https://obot.example.com",
				"tokenValid": true,
				"setupComplete": true
			}`),
		},
	}

	status, err := client.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.Version != "1.2.3" {
		t.Fatalf("version = %q", status.Version)
	}
	if status.DefaultURL != "https://obot.example.com" {
		t.Fatalf("defaultURL = %q", status.DefaultURL)
	}
	if !status.TokenValid || !status.SetupComplete {
		t.Fatalf("expected valid token and complete setup: %#v", status)
	}
}

func TestDetectAgentsParsesCLIJSON(t *testing.T) {
	client := Client{
		Path: "/tmp/obot",
		Runner: fakeRunner{
			wantPath: "/tmp/obot",
			wantArgs: []string{"setup", "detect-agents", "--json"},
			out: []byte(`{
				"agents": [
					{"id": "claude-code", "displayName": "Claude Code", "state": "present", "reason": ""},
					{"id": "cursor", "displayName": "Cursor", "state": "missing", "reason": "not found"}
				]
			}`),
		},
	}

	result, err := client.DetectAgents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Agents) != 2 {
		t.Fatalf("agents len = %d", len(result.Agents))
	}
	if result.Agents[0].ID != "claude-code" || result.Agents[0].State != "present" {
		t.Fatalf("unexpected first agent: %#v", result.Agents[0])
	}
}
