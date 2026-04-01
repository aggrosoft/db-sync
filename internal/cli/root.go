package cli

import (
	"github.com/spf13/cobra"
)

func NewRootCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "db-sync",
		Short:        "Analyze and run database sync operations from environment variables",
		Example:      "db-sync analyze --env-file .env\ndb-sync run --dry-run --env-file .env",
		Version:      Version,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			envFile, err := cmd.Flags().GetString("env-file")
			if err != nil {
				return err
			}
			return app.SetEnvFile(envFile)
		},
	}
	cmd.PersistentFlags().String("env-file", "", "Path to a .env file containing DB_SYNC_* settings")
	cmd.AddCommand(newAnalyzeCommand(app))
	cmd.AddCommand(newRunCommand(app))
	cmd.AddCommand(newVersionCommand())
	return cmd
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the db-sync version",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := cmd.OutOrStdout().Write([]byte(cmd.Root().Version + "\n"))
			return err
		},
	}
}

func newAnalyzeCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "analyze",
		Short: "Compare the selected source and target schemas",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.AnalyzeFromEnvironment(cmd.Context())
		},
	}
}

func newRunCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute the configured sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, err := cmd.Flags().GetBool("dry-run")
			if err != nil {
				return err
			}
			return app.RunFromEnvironment(cmd.Context(), dryRun)
		},
	}
	cmd.Flags().Bool("dry-run", false, "Preview inserts without writing to the target database")
	return cmd
}
