package main

import (
	"github.com/obot-platform/obot/packaging/ObotSetup/internal/cli"
	"github.com/obot-platform/obot/packaging/ObotSetup/internal/ui"
)

var appVersion = "dev"

func main() {
	ui.Run(ui.Config{
		AppVersion: appVersion,
		CLIPath:    cli.DefaultPath(),
	})
}
