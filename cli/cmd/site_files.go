package cmd

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func siteFilesCommand(serverURL, username, password *string) *cobra.Command {
	return &cobra.Command{
		Use:   "files <slug> [prefix]",
		Short: "List files published for a site",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(*serverURL, *username, *password)
			if err != nil {
				return err
			}
			path := fmt.Sprintf("/api/sites/%s/files", url.PathEscape(args[0]))
			if len(args) == 2 {
				path += "?prefix=" + url.QueryEscape(args[1])
			}
			var files []siteFileInfo
			if err := c.doJSON(http.MethodGet, path, nil, &files); err != nil {
				return err
			}
			for _, file := range files {
				fmt.Fprintf(cmd.OutOrStdout(), "%-48s %d\n", file.Path, file.Size)
			}
			return nil
		},
	}
}
