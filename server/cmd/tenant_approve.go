package cmd

import (
	"fmt"
	"io"

	"github.com/csweichel/flink/server/api"

	"github.com/spf13/cobra"
)

func newTenantApproveCommand(configPath *string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "approve <username>",
		Short: "Approve a pending tenant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				tenant, err := store.ApproveTenant(args[0])
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "approved %s\n", tenant.Username)
				return nil
			})
		},
	}
}
