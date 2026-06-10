package cmd

import (
	"fmt"
	"io"
	"time"

	"github.com/csweichel/flink/server/api"

	"github.com/spf13/cobra"
)

func newTenantGetCommand(configPath *string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "get <username>",
		Short: "Show tenant details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				tenant, err := store.ReadTenant(args[0])
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "%-24s %-10s created=%s updated=%s\n", tenant.Username, tenant.Status, tenant.CreatedAt.Format(time.RFC3339), tenant.UpdatedAt.Format(time.RFC3339))
				return nil
			})
		},
	}
}
