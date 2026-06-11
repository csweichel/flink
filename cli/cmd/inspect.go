package cmd

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func inspectCommand(ctx *commandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect [site]",
		Short: "Show site metadata and resources",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, config, err := ctx.client()
			if err != nil {
				return err
			}
			site := config.Site
			if len(args) == 1 {
				site = args[0]
			}
			if site == "" {
				return fmt.Errorf("missing site; pass a site or run inside a project with .flink/site.json")
			}
			var details siteDetails
			if err := c.doJSON(http.MethodGet, "/api/sites/"+url.PathEscape(site), nil, &details); err != nil {
				return err
			}
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, details)
			}
			printSections(cmd.OutOrStdout(), "Site details",
				outputSection{Title: "Target", Rows: []outputRow{
					row("Site", details.Site.Slug),
					row("Tenant", config.Tenant),
					row("URL", canonicalSiteURL(config.Server, config.Tenant, details.Site.Slug)),
				}},
				outputSection{Title: "Access", Rows: []outputRow{
					row("Mode", formatSiteAuthPolicy(details.Site.Auth)),
				}},
				outputSection{Title: "Resources", Rows: []outputRow{
					row("Files", len(details.Files)),
					row("Uploads", len(details.Uploads)),
					row("State keys", len(details.Data)),
					row("Size", formatBytes(details.Site.TotalBytes)),
				}},
				outputSection{Title: "Capabilities", Rows: []outputRow{
					row("Detected", joinCapabilities(details.Site.Capabilities)),
				}},
			)
			if len(details.Files) == 0 {
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "Files")
			for _, file := range details.Files {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-48s %s\n", file.Path, formatBytes(file.Size))
			}
			return nil
		},
	}
}
