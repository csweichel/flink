package cmd

import (
	"io"

	"github.com/csweichel/flink/server/api"

	"github.com/spf13/cobra"
)

func newTenantPendingCommand(configPath *string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "pending",
		Short: "List pending tenant registrations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				return printTenants(out, store, api.TenantPending)
			})
		},
	}
}
