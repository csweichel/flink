package cmd

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func siteDeleteFileCommand(serverURL, username, password *string) *cobra.Command {
	return &cobra.Command{
		Use:     "delete-file <slug> <site-path>",
		Aliases: []string{"rm"},
		Short:   "Delete one published file from a site",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(*serverURL, *username, *password)
			if err != nil {
				return err
			}
			path := fmt.Sprintf("/api/sites/%s/files?path=%s", url.PathEscape(args[0]), url.QueryEscape(args[1]))
			var out map[string]bool
			if err := c.doJSON(http.MethodDelete, path, nil, &out); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s from %s\n", args[1], args[0])
			return nil
		},
	}
}
