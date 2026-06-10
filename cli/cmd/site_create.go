package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

func siteCreateCommand(serverURL, username, password *string) *cobra.Command {
	return &cobra.Command{
		Use:   "create <slug>",
		Short: "Create a site on the Flink server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(*serverURL, *username, *password)
			if err != nil {
				return err
			}
			var meta siteMeta
			if err := c.doJSON(http.MethodPost, "/api/sites", map[string]string{"slug": args[0]}, &meta); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created %s at %s\n", meta.Slug, c.siteURL(meta.Slug))
			return nil
		},
	}
}
