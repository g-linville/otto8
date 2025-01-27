package cli

import (
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

type EmailReceivers struct {
	root   *Otto8
	Quiet  bool   `usage:"Only print IDs of agents" short:"q"`
	Wide   bool   `usage:"Print more information" short:"w"`
	Output string `usage:"Output format (table, json, yaml)" short:"o" default:"table"`
}

func (l *EmailReceivers) Customize(cmd *cobra.Command) {
	cmd.Aliases = []string{"emailreceiver", "er"}
}

func (l *EmailReceivers) Run(cmd *cobra.Command, _ []string) error {
	ers, err := l.root.Client.ListEmailReceivers(cmd.Context())
	if err != nil {
		return err
	}

	if ok, err := output(l.Output, ers); ok || err != nil {
		return err
	}

	if l.Quiet {
		for _, emailReceiver := range ers.Items {
			fmt.Println(emailReceiver.ID)
		}
		return nil
	}

	w := newTable("ID", "NAME", "DESCRIPTION", "WORKFLOW", "ADDRESS", "CREATED")
	for _, er := range ers.Items {
		w.WriteRow(er.ID, er.Name, truncate(er.Description, l.Wide), er.Workflow,
			er.EmailAddress,
			humanize.Time(er.Created.Time))
	}

	return w.Err()
}
