package cmd

import (
	"fmt"
	"io"

	"github.com/csweichel/flink/server/api"

	"github.com/spf13/cobra"
)

func newTenantBootstrapCommand(configPath *string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "bootstrap <username> <password>",
		Short: "Create or update an approved tenant for automation",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				tenant, err := store.CreateApprovedTenant(args[0], args[1])
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "bootstrapped %s\n", tenant.Username)
				return nil
			})
		},
	}
}
