package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func configCommand(ctx *commandContext) *cobra.Command {
	config := &cobra.Command{Use: "config", Short: "Manage Flink CLI config"}
	config.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a default config value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			user := readUserConfig()
			switch args[0] {
			case "server":
				user.Server = strings.TrimRight(args[1], "/")
			case "tenant":
				user.Tenant = strings.ToLower(strings.TrimSpace(args[1]))
			default:
				return fmt.Errorf("unknown config key %q", args[0])
			}
			if err := writeUserConfig(user); err != nil {
				return err
			}
			printSections(cmd.OutOrStdout(), "Config updated",
				outputSection{Title: "Value", Rows: []outputRow{
					row(configLabel(args[0]), args[1]),
				}},
			)
			return nil
		},
	})
	config.AddCommand(&cobra.Command{
		Use:   "login",
		Short: "Save current server and tenant defaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := ctx.resolveConfig()
			if resolved.Server == "" || resolved.Tenant == "" {
				return fmt.Errorf("set --server and --tenant or FLINK_SERVER and FLINK_TENANT before login")
			}
			user := readUserConfig()
			user.Server = resolved.Server
			user.Tenant = resolved.Tenant
			if resolved.Password != "" {
				user.Password = resolved.Password
			}
			if err := writeUserConfig(user); err != nil {
				return err
			}
			printSections(cmd.OutOrStdout(), "Login saved",
				outputSection{Title: "Defaults", Rows: []outputRow{
					row("Server", user.Server),
					row("Tenant", user.Tenant),
				}},
			)
			return nil
		},
	})
	config.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show resolved config",
		RunE: func(cmd *cobra.Command, args []string) error {
			resolved := ctx.resolveConfig()
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, map[string]string{
					"site":   resolved.Site,
					"server": resolved.Server,
					"tenant": resolved.Tenant,
				})
			}
			printSections(cmd.OutOrStdout(), "Resolved config",
				outputSection{Title: "Defaults", Rows: []outputRow{
					row("Server", resolved.Server),
					row("Tenant", resolved.Tenant),
					row("Site", resolved.Site),
				}},
			)
			return nil
		},
	})
	return config
}

func configLabel(key string) string {
	if key == "" {
		return ""
	}
	return strings.ToUpper(key[:1]) + key[1:]
}
