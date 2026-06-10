package cmd

import (
	"fmt"
	"io"

	"github.com/csweichel/flink/server/api"

	"github.com/spf13/cobra"
)

func newTenantListCommand(configPath *string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list [pending|approved|denied|all]",
		Short: "List tenants",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			status := ""
			if len(args) == 1 && args[0] != "all" {
				switch args[0] {
				case api.TenantPending, api.TenantApproved, api.TenantDenied:
					status = args[0]
				default:
					return fmt.Errorf("unknown tenant status %q", args[0])
				}
			}
			return withTenantStore(*configPath, func(store *api.Store) error {
				return printTenants(out, store, status)
			})
		},
	}
}
