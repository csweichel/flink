package cmd

import (
	"fmt"
	"io"

	"github.com/csweichel/flink/server/api"

	"github.com/spf13/cobra"
)

func newTenantCreateCommand(configPath *string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "create <username> <password>",
		Short: "Create a new approved tenant",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				tenant, err := store.RegisterApprovedTenant(args[0], args[1])
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "created %s\n", tenant.Username)
				return nil
			})
		},
	}
}
