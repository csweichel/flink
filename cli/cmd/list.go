package cmd

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func listCommand(ctx *commandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sites",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, config, err := ctx.client()
			if err != nil {
				return err
			}
			var sites []siteMeta
			if err := c.doJSON(http.MethodGet, "/api/sites", nil, &sites); err != nil {
				return err
			}
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, sites)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Sites")
			if len(sites) == 0 {
				fmt.Fprintln(cmd.OutOrStdout())
				fmt.Fprintln(cmd.OutOrStdout(), "No sites found.")
				return nil
			}
			for _, site := range sites {
				printSections(cmd.OutOrStdout(), "",
					outputSection{Title: site.Slug, Rows: []outputRow{
						row("Access", formatSiteAuthPolicy(site.Auth)),
						row("Updated", formatTime(site.UpdatedAt)),
						row("Files", site.FileCount),
						row("Size", formatBytes(site.TotalBytes)),
						row("URL", canonicalSiteURL(config.Server, config.Tenant, site.Slug)),
					}},
				)
			}
			return nil
		},
	}
}

func formatBytes(value int) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	if value < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(value)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(value)/(1024*1024))
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format(time.RFC3339)
}

func joinCapabilities(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ", ")
}
