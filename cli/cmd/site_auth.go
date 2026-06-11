package cmd

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func siteAuthCommand(serverURL, username, password *string) *cobra.Command {
	return &cobra.Command{
		Use:   "auth <slug> [owner|none|tenants [tenant...]]",
		Short: "Show or change who can view and use a site",
		Long: strings.TrimSpace(`Show or change who can view and use a site.

Modes:
  owner            only the tenant that owns the site can view and use it
  none             anyone can view and use the site's browser APIs
  tenants          signed-in tenants can view and use the site's browser APIs
                   with no tenant list, any signed-in tenant is allowed
                   with a tenant list, only those tenants are allowed

Examples:
  flink site auth demo
  flink site auth demo owner
  flink site auth demo none
  flink site auth demo tenants
  flink site auth demo tenants alice bob`),
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return nil
			}
			if len(args) < 2 {
				return fmt.Errorf("requires a site slug")
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
			c, err := newClient(*serverURL, *username, *password)
			if err != nil {
				return err
			}
			path := "/api/sites/" + args[0] + "/auth"
			var policy siteAuthPolicy
			if len(args) == 1 {
				if err := c.doJSON(http.MethodGet, path, nil, &policy); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s auth: %s\n", args[0], formatSiteAuthPolicy(policy))
				return nil
			}
			policy = siteAuthPolicy{Mode: args[1]}
			if args[1] == "tenants" {
				policy.Tenants = append([]string(nil), args[2:]...)
				sort.Strings(policy.Tenants)
			}
			if err := c.doJSON(http.MethodPut, path, policy, &policy); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s auth: %s\n", args[0], formatSiteAuthPolicy(policy))
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
