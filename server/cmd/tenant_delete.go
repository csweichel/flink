package cmd

import (
	"fmt"
	"io"

	"github.com/csweichel/flink/server/api"

	"github.com/spf13/cobra"
)

func newTenantDeleteCommand(configPath *string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <username>",
		Short: "Delete a tenant and all of its sites",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				if err := store.DeleteTenant(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(out, "deleted %s\n", args[0])
				return nil
			})
		},
	}
}
