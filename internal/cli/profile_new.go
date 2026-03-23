package cli

import "github.com/spf13/cobra"

func newProfileNewCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "new",
		Short: "Launch the interactive profile creation flow",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.RunProfileNew(cmd.Context())
		},
	}
}
