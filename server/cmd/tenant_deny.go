package cmd

import (
	"fmt"
	"io"

	"github.com/csweichel/flink/server/api"

	"github.com/spf13/cobra"
)

func newTenantDenyCommand(configPath *string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "deny <username>",
		Short: "Deny a tenant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				tenant, err := store.DenyTenant(args[0])
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "denied %s\n", tenant.Username)
				return nil
			})
		},
	}
}
