package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func authCommand(ctx *commandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "auth <site> [owner|none|tenants [tenant...]]",
		Short: "Show or change site access",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return nil
			}
			if len(args) < 2 {
				return fmt.Errorf("requires a site")
			}
			switch args[1] {
			case "owner", "none":
				if len(args) != 2 {
					return fmt.Errorf("mode %q does not accept tenant names", args[1])
				}
			case "tenants":
			default:
				return fmt.Errorf("unknown auth mode %q", args[1])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			path := "/api/sites/" + url.PathEscape(args[0]) + "/auth"
			var policy siteAuthPolicy
			if len(args) == 1 {
				if err := c.doJSON(http.MethodGet, path, nil, &policy); err != nil {
					return err
				}
			} else {
				policy = siteAuthPolicy{Mode: args[1]}
				if args[1] == "tenants" {
					policy.Tenants = append([]string(nil), args[2:]...)
					sort.Strings(policy.Tenants)
				}
				if err := c.doJSON(http.MethodPut, path, policy, &policy); err != nil {
					return err
				}
			}
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, policy)
			}
			printSections(cmd.OutOrStdout(), "Access policy",
				outputSection{Title: "Target", Rows: []outputRow{
					row("Site", args[0]),
				}},
				outputSection{Title: "Access", Rows: []outputRow{
					row("Mode", formatSiteAuthPolicy(policy)),
				}},
			)
			return nil
		},
	}
}

func formatSiteAuthPolicy(policy siteAuthPolicy) string {
	switch policy.Mode {
	case "owner":
		return "owner"
	case "none":
		return "none (public)"
	case "tenants":
		if len(policy.Tenants) == 0 {
			return "tenants (any tenant)"
		}
		return "tenants (" + strings.Join(policy.Tenants, ", ") + ")"
	default:
		return strings.TrimSpace(policy.Mode)
	}
}
