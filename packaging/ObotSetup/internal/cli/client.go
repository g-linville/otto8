package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

type Runner interface {
	Run(ctx context.Context, path string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, path string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s %v failed: %s", path, args, msg)
	}
	return out, nil
}

type Client struct {
	Path        string
	Runner      Runner
	SetupRunner SetupRunner
}

func (c Client) Status(ctx context.Context) (Status, error) {
	var status Status
	if err := c.decode(ctx, &status, "setup", "status", "--json"); err != nil {
		return Status{}, err
	}
	return status, nil
}

func (c Client) DetectAgents(ctx context.Context) (DetectAgentsResult, error) {
	var result DetectAgentsResult
	if err := c.decode(ctx, &result, "setup", "detect-agents", "--json"); err != nil {
		return DetectAgentsResult{}, err
	}
	return result, nil
}

func (c Client) Exists() bool {
	if c.Path == "" {
		return false
	}
	info, err := os.Stat(c.Path)
	return err == nil && !info.IsDir()
}

func (c Client) decode(ctx context.Context, target any, args ...string) error {
	runner := c.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	path := c.Path
	if path == "" {
		path = DefaultPath()
	}

	out, err := runner.Run(ctx, path, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, target); err != nil {
		return fmt.Errorf("parse CLI JSON: %w", err)
	}
	return nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, exec.ErrNotFound)
}
