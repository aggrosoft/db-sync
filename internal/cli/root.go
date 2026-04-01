package cli

import (
	"github.com/spf13/cobra"
)

func NewRootCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "db-sync",
		Short:   "Validate database sync configuration from environment variables",
		Example: "db-sync --env-file .env",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.RunFromEnvironment(cmd.Context())
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
	return cmd
}
