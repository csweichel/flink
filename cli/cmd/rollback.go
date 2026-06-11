package cmd

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func rollbackCommand(ctx *commandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "rollback <site> [version]",
		Short: "Restore a previous publish",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			body := map[string]string{}
			if len(args) == 2 {
				body["version"] = args[1]
			}
			var record publishRecord
			if err := c.doJSON(http.MethodPost, "/api/sites/"+url.PathEscape(args[0])+"/rollback", body, &record); err != nil {
				return err
			}
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, record)
			}
			printSections(cmd.OutOrStdout(), "Rollback complete",
				outputSection{Title: "Target", Rows: []outputRow{
					row("Site", args[0]),
					row("Restored", record.RollbackOf),
					row("Version", record.ID),
				}},
				outputSection{Title: "Result", Rows: []outputRow{
					row("Files", record.FileCount),
					row("Size", formatBytes(record.TotalBytes)),
				}},
			)
			return nil
		},
	}
}
