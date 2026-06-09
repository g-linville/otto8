package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/cli/internal"
	"github.com/obot-platform/obot/pkg/localagents"
	"github.com/spf13/cobra"
)

type Audit struct {
	Client string `usage:"Local agent client ID"`
	Event  string `usage:"Client hook event ID"`

	root       *Obot
	auditToken func(string) (string, error)
	submit     func(context.Context, *apiclient.Client, types.LocalAgentAuditLogIngest) error // for unit testing purposes
}

func (a *Audit) Customize(cmd *cobra.Command) {
	cmd.Use = "audit"
	cmd.Short = "Submit a local agent tool-call audit event"
	cmd.Hidden = true
	cmd.Args = cobra.NoArgs
}

func (a *Audit) Run(cmd *cobra.Command, _ []string) error {
	if strings.TrimSpace(a.Client) == "" {
		return fmt.Errorf("--client is required")
	}
	if strings.TrimSpace(a.Event) == "" {
		return fmt.Errorf("--event is required")
	}

	data, truncated, err := readAuditStdin(cmd.InOrStdin())
	if err != nil {
		a.debugf(cmd, "read audit event: %v\n", err)
		return nil
	}

	auditLog, err := localagents.NormalizeAuditEvent(localagents.AuditNormalizeOptions{
		ClientID:       a.Client,
		HookEvent:      a.Event,
		Payload:        data,
		InputTruncated: truncated,
	})
	if err != nil {
		a.debugf(cmd, "normalize audit event: %v\n", err)
		return nil
	}

	if a.root == nil {
		a.debugf(cmd, "submit audit event: root command is not configured\n")
		return nil
	}

	client := a.root.Client
	if client == nil {
		a.debugf(cmd, "submit audit event: API client is not configured\n")
		return nil
	}

	tokenFn := a.auditToken
	if tokenFn == nil {
		tokenFn = internal.AuditToken
	}
	token, err := tokenFn(client.BaseURL)
	if err != nil {
		a.debugf(cmd, "submit audit event: %v\n", err)
		return nil
	}

	submitter := a.submit
	if submitter == nil {
		submitter = func(ctx context.Context, client *apiclient.Client, auditLog types.LocalAgentAuditLogIngest) error {
			_, err := client.SubmitLocalAgentAuditLog(ctx, auditLog)
			return err
		}
	}
	if err := submitter(cmd.Context(), client.WithToken(token), auditLog); err != nil {
		a.debugf(cmd, "submit audit event: %v\n", err)
		return nil
	}

	return nil
}

func readAuditStdin(r io.Reader) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(r, localagents.MaxAuditEventBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) > localagents.MaxAuditEventBytes {
		return data[:localagents.MaxAuditEventBytes], true, nil
	}
	return data, false, nil
}

func (a *Audit) debugf(cmd *cobra.Command, format string, args ...any) {
	if !a.debugEnabled() {
		return
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), format, args...)
}

func (a *Audit) debugEnabled() bool {
	return a.root != nil && a.root.Debug || os.Getenv("OBOT_AUDIT_DEBUG") == "1"
}
