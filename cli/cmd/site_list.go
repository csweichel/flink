package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

func siteListCommand(serverURL, username, password *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sites on the Flink server",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(*serverURL, *username, *password)
			if err != nil {
				return err
			}
			var sites []siteMeta
			if err := c.doJSON(http.MethodGet, "/api/sites", nil, &sites); err != nil {
				return err
			}
			for _, s := range sites {
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-28s %s\n", s.Slug, s.UpdatedAt.Format(time.RFC3339), c.siteURL(s.Slug))
			}
			return nil
		},
	}
}
