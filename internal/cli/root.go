package cli

import (
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type interactivityProbe struct {
	stdin      io.Reader
	isTerminal func(fd int) bool
	lookupEnv  func(string) string
}

func defaultInteractivityProbe(stdin io.Reader) interactivityProbe {
	return interactivityProbe{
		stdin:      stdin,
		isTerminal: term.IsTerminal,
		lookupEnv:  os.Getenv,
	}
}

func (probe interactivityProbe) isInteractive() bool {
	if probe.isTerminal == nil {
		probe.isTerminal = term.IsTerminal
	}
	if probe.lookupEnv == nil {
		probe.lookupEnv = os.Getenv
	}
	if probe.isHeadless() {
		return false
	}
	stdinIsCharDevice, known := probe.stdinIsCharacterDevice()
	if known && !stdinIsCharDevice {
		return false
	}
	if probe.isTerminal(0) {
		return true
	}
	if known && stdinIsCharDevice {
		return true
	}
	return probe.hasTerminalSignal()
}

func (probe interactivityProbe) stdinIsCharacterDevice() (bool, bool) {
	type statFile interface {
		Stat() (fs.FileInfo, error)
	}
	file, ok := probe.stdin.(statFile)
	if !ok {
		return false, false
	}
	info, err := file.Stat()
	if err != nil {
		return false, false
	}
	return info.Mode()&fs.ModeCharDevice != 0, true
}

func (probe interactivityProbe) hasTerminalSignal() bool {
	for _, key := range []string{"WT_SESSION", "ConEmuPID", "ANSICON"} {
		if strings.TrimSpace(probe.lookupEnv(key)) != "" {
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(probe.lookupEnv("TERM_PROGRAM")), "vscode") {
		return true
	}
	return strings.TrimSpace(probe.lookupEnv("TERM")) != ""
}

func (probe interactivityProbe) isHeadless() bool {
	for _, key := range []string{"CI", "GITHUB_ACTIONS", "TF_BUILD", "JENKINS_URL", "TEAMCITY_VERSION"} {
		if strings.TrimSpace(probe.lookupEnv(key)) != "" {
			return true
		}
	}
	return false
}

func NewRootCommand(app *App) *cobra.Command {
	probe := defaultInteractivityProbe(app.stdin)
	cmd := &cobra.Command{
		Use:   "db-sync",
		Short: "Create, validate, and reuse safe database sync profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !probe.isInteractive() {
				return cmd.Help()
			}
			return app.StartInteractiveProfile(cmd.Context())
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			envFile, err := cmd.Flags().GetString("env-file")
			if err != nil {
				return err
			}
			return app.SetEnvFile(envFile)
		},
	}
	cmd.PersistentFlags().String("env-file", "", "Path to a .env file used to resolve DSN placeholders")
	cmd.AddCommand(NewProfileCommand(app))
	return cmd
}
