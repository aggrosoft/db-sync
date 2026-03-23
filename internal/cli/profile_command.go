package cli

import "github.com/spf13/cobra"

func NewProfileCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Create, edit, list, and validate saved profiles",
		Example: "db-sync profile new\n" +
			"db-sync profile edit <name>\n" +
			"db-sync profile list\n" +
			"db-sync profile validate <name>",
	}
	cmd.AddCommand(newProfileNewCommand(app))
	cmd.AddCommand(newProfileEditCommand(app))
	cmd.AddCommand(newProfileListCommand(app))
	cmd.AddCommand(newProfileValidateCommand(app))
	return cmd
}

func newProfileListCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.RunProfileList(cmd.Context())
		},
	}
}
