//go:build !darwin && !windows

package ui

import (
	"fmt"

	"github.com/obot-platform/obot/packaging/ObotSetup/internal/cli"
)

type Config struct {
	AppVersion string
	CLIPath    string
	Client     cli.Client
}

func Run(cfg Config) {
	fmt.Printf("Obot Setup GUI is only supported on macOS. CLI path: %s\n", cfg.CLIPath)
}
