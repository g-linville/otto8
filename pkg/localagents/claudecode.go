package localagents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/obot-platform/obot/pkg/devicescan"
	"github.com/obot-platform/obot/pkg/localagents/assets"
)

const (
	ClaudeCodeAgentID     = "claude-code"
	claudeCodeDisplayName = "Claude Code"
)

type ClaudeCode struct {
	home string
}

func NewClaudeCode() ClaudeCode {
	return ClaudeCode{}
}

func (c ClaudeCode) ID() string {
	return ClaudeCodeAgentID
}

func (c ClaudeCode) DisplayName() string {
	return claudeCodeDisplayName
}

func (c ClaudeCode) Detect(ctx context.Context) DetectionResult {
	result := DetectionResult{
		AgentID:     c.ID(),
		DisplayName: c.DisplayName(),
		State:       DetectionMissing,
	}
	if err := ctx.Err(); err != nil {
		result.Reason = err.Error()
		return result
	}

	home, err := resolveHome("", c.home)
	if err != nil {
		result.Reason = err.Error()
		return result
	}

	presence := devicescan.DetectClaudeCodePresence(home)
	switch {
	case presence.BinaryPath != "":
		result.State = DetectionPresent
		result.Reason = "found claude binary at " + presence.BinaryPath
	case presence.ConfigPath != "":
		result.State = DetectionPresent
		result.Reason = "found Claude Code config at " + presence.ConfigPath
	case presence.InstallPath != "":
		result.State = DetectionPresent
		result.Reason = "found Claude Code install at " + presence.InstallPath
	default:
		result.Reason = "Claude Code was not detected"
	}

	return result
}

func (c ClaudeCode) InstallBootstrap(ctx context.Context, home string) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	home, err := resolveHome(home, c.home)
	if err != nil {
		return InstallResult{}, err
	}

	rendered, err := assets.RenderAgentSkills(assets.ClaudeCodeTemplateData())
	if err != nil {
		return InstallResult{}, err
	}

	installed, err := installBootstrapAssets(claudeCodeSkillsRoot(home), rendered)
	if err != nil {
		return InstallResult{}, err
	}
	hookInstalled, err := installClaudeCodeAuditHooks(home, obotHookCommand())
	if err != nil {
		return InstallResult{}, err
	}
	if hookInstalled != "" {
		installed = append(installed, hookInstalled)
	}

	return InstallResult{
		AgentID:     c.ID(),
		DisplayName: c.DisplayName(),
		Installed:   installed,
		Message:     "Installed Obot bootstrap skills and audit hooks for Claude Code",
	}, nil
}

func (c ClaudeCode) InstallSkill(ctx context.Context, home string, skill SkillArchive) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	home, err := resolveHome(home, c.home)
	if err != nil {
		return InstallResult{}, err
	}
	name, installed, err := installSkillArchiveToRoot(claudeCodeSkillsRoot(home), skill)
	if err != nil {
		return InstallResult{}, err
	}

	return InstallResult{
		AgentID:     c.ID(),
		DisplayName: c.DisplayName(),
		Installed:   installed,
		Message:     fmt.Sprintf("Installed %s for Claude Code", name),
	}, nil
}

func claudeCodeSkillsRoot(home string) string {
	return filepath.Join(home, ".claude", "skills")
}

func claudeCodeSettingsPath(home string) string {
	return filepath.Join(home, ".claude", "settings.json")
}

func obotHookCommand() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "obot"
	}
	return strconv.Quote(exe) + " audit claude-code-hook"
}

type claudeHookMatcher struct {
	Matcher string              `json:"matcher,omitempty"`
	Hooks   []claudeCommandHook `json:"hooks"`
}

type claudeCommandHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func installClaudeCodeAuditHooks(home, command string) (string, error) {
	settingsPath := claudeCodeSettingsPath(home)
	settings := map[string]any{}
	if data, err := os.ReadFile(settingsPath); err == nil {
		if len(data) > 0 {
			if err := json.Unmarshal(data, &settings); err != nil {
				return "", fmt.Errorf("parse %s: %w", settingsPath, err)
			}
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", settingsPath, err)
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}

	changed := false
	for _, event := range []string{"PostToolUse", "PostToolUseFailure"} {
		eventChanged, err := ensureClaudeCodeAuditHook(hooks, event, command)
		if err != nil {
			return "", fmt.Errorf("update Claude Code %s hook: %w", event, err)
		}
		changed = changed || eventChanged
	}
	if !changed {
		return "", nil
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return "", fmt.Errorf("create Claude Code config directory: %w", err)
	}
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return "", fmt.Errorf("write %s: %w", settingsPath, err)
	}
	return settingsPath, nil
}

func ensureClaudeCodeAuditHook(hooks map[string]any, event, command string) (bool, error) {
	var matchers []claudeHookMatcher
	if raw, ok := hooks[event]; ok {
		data, err := json.Marshal(raw)
		if err != nil {
			return false, err
		}
		if err := json.Unmarshal(data, &matchers); err != nil {
			return false, err
		}
	}

	hook := claudeCommandHook{Type: "command", Command: command}
	for i := range matchers {
		if matchers[i].Matcher != "*" {
			continue
		}
		for _, existing := range matchers[i].Hooks {
			if existing.Type == hook.Type && existing.Command == hook.Command {
				hooks[event] = matchers
				return false, nil
			}
		}
		matchers[i].Hooks = append(matchers[i].Hooks, hook)
		hooks[event] = matchers
		return true, nil
	}

	matchers = append(matchers, claudeHookMatcher{
		Matcher: "*",
		Hooks:   []claudeCommandHook{hook},
	})
	hooks[event] = matchers
	return true, nil
}
