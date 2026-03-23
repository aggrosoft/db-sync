package cli

import "github.com/spf13/cobra"

func newProfileValidateCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "validate <name>",
		Short: "Validate a saved profile without exposing resolved secrets",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.RunProfileValidate(cmd.Context(), args[0])
		},
	}
}
