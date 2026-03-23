package cli

import "github.com/spf13/cobra"

func newProfileEditCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit a saved profile with the same guided flow used for creation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.RunProfileEdit(cmd.Context(), args[0])
		},
	}
}
