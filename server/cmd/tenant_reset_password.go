package cmd

import (
	"fmt"
	"io"

	"github.com/csweichel/flink/server/api"

	"github.com/spf13/cobra"
)

func newTenantResetPasswordCommand(configPath *string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "reset-password <username> <password>",
		Short: "Reset a tenant password",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				tenant, err := store.ResetTenantPassword(args[0], args[1])
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "reset password for %s\n", tenant.Username)
				return nil
			})
		},
	}
}
