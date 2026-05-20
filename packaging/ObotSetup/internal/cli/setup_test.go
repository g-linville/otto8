package cli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeSetupRunner struct {
	wantPath string
	wantArgs []string
}

func (f fakeSetupRunner) RunSetup(_ context.Context, path string, args []string, onEvent func(SetupProgressEvent)) error {
	if path != f.wantPath {
		return errors.New("unexpected path")
	}
	if !reflect.DeepEqual(args, f.wantArgs) {
		return errors.New("unexpected args")
	}
	if onEvent != nil {
		onEvent(SetupProgressEvent{Type: SetupProgressComplete, URL: "https://obot.example.com"})
	}
	return nil
}

func TestBuildSetupArgsUsesExplicitAgentSelection(t *testing.T) {
	got := BuildSetupArgs(SetupOptions{
		URL:      " https://obot.example.com ",
		AgentIDs: []string{"claude-code", "cursor"},
	})
	want := []string{
		"setup",
		"--url", "https://obot.example.com",
		"--agents", "claude-code,cursor",
		"--yes",
		"--non-interactive",
		"--output", "json",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestBuildSetupArgsUsesNoneWhenNoAgentsSelected(t *testing.T) {
	got := BuildSetupArgs(SetupOptions{URL: "https://obot.example.com"})
	if got[4] != "none" {
		t.Fatalf("--agents = %q, want none", got[4])
	}
}

func TestRunSetupUsesConfiguredRunner(t *testing.T) {
	client := Client{
		Path: "/tmp/obot",
		SetupRunner: fakeSetupRunner{
			wantPath: "/tmp/obot",
			wantArgs: BuildSetupArgs(SetupOptions{
				URL:      "https://obot.example.com",
				AgentIDs: []string{"cursor"},
			}),
		},
	}

	var complete bool
	err := client.RunSetup(t.Context(), SetupOptions{
		URL:      "https://obot.example.com",
		AgentIDs: []string{"cursor"},
	}, func(event SetupProgressEvent) {
		complete = event.Type == SetupProgressComplete
	})
	if err != nil {
		t.Fatal(err)
	}
	if !complete {
		t.Fatal("expected complete event")
	}
}

func TestParseSetupProgress(t *testing.T) {
	input := strings.NewReader(`{"type":"auth_started","url":"https://obot.example.com"}` + "\n" +
		`{"type":"agent_installed","agentID":"claude-code","displayName":"Claude Code","installed":["/tmp/file"]}` + "\n")

	var events []SetupProgressEvent
	if err := ParseSetupProgress(input, func(event SetupProgressEvent) {
		events = append(events, event)
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d", len(events))
	}
	if events[1].AgentID != "claude-code" || len(events[1].Installed) != 1 {
		t.Fatalf("unexpected agent event: %#v", events[1])
	}
}

func TestSetupErrorDisplayMessageMapsStructuredCodes(t *testing.T) {
	got := SetupErrorDisplayMessage(SetupProgressEvent{
		Type:    SetupProgressError,
		Code:    "auth_timeout",
		Message: "context deadline exceeded",
	})
	if !strings.Contains(got, "browser login timed out") {
		t.Fatalf("message = %q", got)
	}
	if !strings.Contains(got, "context deadline exceeded") {
		t.Fatalf("message omitted detail: %q", got)
	}
}
