package cmd

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func siteDeleteCommand(serverURL, username, password *string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <slug>",
		Short: "Delete a site from the Flink server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(*serverURL, *username, *password)
			if err != nil {
				return err
			}
			var out map[string]bool
			if err := c.doJSON(http.MethodDelete, "/api/sites/"+url.PathEscape(args[0]), nil, &out); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", args[0])
			return nil
		},
	}
}
