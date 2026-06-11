package cmd

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func historyCommand(ctx *commandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "history <site>",
		Short: "List publish history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			var records []publishRecord
			if err := c.doJSON(http.MethodGet, "/api/sites/"+url.PathEscape(args[0])+"/publishes", nil, &records); err != nil {
				return err
			}
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, records)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Publish history for %s\n", args[0])
			if len(records) == 0 {
				fmt.Fprintln(cmd.OutOrStdout())
				fmt.Fprintln(cmd.OutOrStdout(), "No publishes recorded.")
				return nil
			}
			for _, record := range records {
				printSections(cmd.OutOrStdout(), "",
					outputSection{Title: record.ID, Rows: []outputRow{
						row("Published", formatTime(record.CreatedAt)),
						row("Source", record.Source),
						row("Files", record.FileCount),
						row("Size", formatBytes(record.TotalBytes)),
					}},
				)
			}
			return nil
		},
	}
}
