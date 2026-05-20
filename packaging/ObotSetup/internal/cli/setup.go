package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type SetupRunner interface {
	RunSetup(ctx context.Context, path string, args []string, onEvent func(SetupProgressEvent)) error
}

type ExecSetupRunner struct{}

type SetupOptions struct {
	URL      string
	AgentIDs []string
}

type SetupProgressEvent struct {
	Type        string   `json:"type"`
	Code        string   `json:"code,omitempty"`
	Message     string   `json:"message,omitempty"`
	URL         string   `json:"url,omitempty"`
	AgentID     string   `json:"agentID,omitempty"`
	DisplayName string   `json:"displayName,omitempty"`
	Installed   []string `json:"installed,omitempty"`
}

const (
	SetupProgressAuthStarted    = "auth_started"
	SetupProgressAuthCompleted  = "auth_completed"
	SetupProgressConfigSaved    = "config_saved"
	SetupProgressAgentInstalled = "agent_installed"
	SetupProgressComplete       = "complete"
	SetupProgressError          = "error"
)

func (c Client) RunSetup(ctx context.Context, opts SetupOptions, onEvent func(SetupProgressEvent)) error {
	runner := c.SetupRunner
	if runner == nil {
		runner = ExecSetupRunner{}
	}
	path := c.Path
	if path == "" {
		path = DefaultPath()
	}
	return runner.RunSetup(ctx, path, BuildSetupArgs(opts), onEvent)
}

func BuildSetupArgs(opts SetupOptions) []string {
	return []string{
		"setup",
		"--url", strings.TrimSpace(opts.URL),
		"--agents", setupAgentsArg(opts.AgentIDs),
		"--yes",
		"--non-interactive",
		"--output", "json",
	}
}

func setupAgentsArg(agentIDs []string) string {
	var selected []string
	for _, id := range agentIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			selected = append(selected, id)
		}
	}
	if len(selected) == 0 {
		return "none"
	}
	return strings.Join(selected, ",")
}

func (ExecSetupRunner) RunSetup(ctx context.Context, path string, args []string, onEvent func(SetupProgressEvent)) error {
	cmd := exec.CommandContext(ctx, path, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open setup output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start setup: %w", err)
	}

	parseErr := ParseSetupProgress(stdout, onEvent)
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if parseErr != nil {
		return parseErr
	}
	if waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return fmt.Errorf("%s %v failed: %s", path, args, msg)
	}
	return nil
}

func ParseSetupProgress(r io.Reader, onEvent func(SetupProgressEvent)) error {
	dec := json.NewDecoder(r)
	for {
		var event SetupProgressEvent
		err := dec.Decode(&event)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("parse setup progress JSON: %w", err)
		}
		if onEvent != nil {
			onEvent(event)
		}
	}
}

func SetupErrorDisplayMessage(event SetupProgressEvent) string {
	detail := strings.TrimSpace(event.Message)
	var action string
	switch event.Code {
	case "invalid_url":
		action = "Check the Obot URL and try again."
	case "server_unreachable":
		action = "Check that the Obot URL is reachable, then try again."
	case "auth_unavailable":
		action = "The server does not have a usable login provider for non-interactive setup."
	case "auth_timeout":
		action = "The browser login timed out. Start setup again when you are ready to log in."
	case "auth_canceled":
		action = "The browser login was canceled. Start setup again to retry."
	case "config_save_failed":
		action = "Obot could not save the local CLI configuration. Check local file permissions and retry."
	case "agent_detection_failed":
		action = "Obot could not verify one of the selected local agents. Refresh detection and retry."
	case "agent_install_failed":
		action = "Obot could not install one or more selected local agent bootstrap files."
	default:
		action = "Setup failed. Review the error and try again."
	}
	if detail == "" {
		return action
	}
	return action + "\n\n" + detail
}
