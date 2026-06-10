package cmd

import (
	"io"

	"github.com/spf13/cobra"
)

func newTenantsCommand(configPath *string, out io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "tenants",
		Short: "Manage approved and pending tenants",
	}
	command.AddCommand(newTenantListCommand(configPath, out))
	command.AddCommand(newTenantPendingCommand(configPath, out))
	command.AddCommand(newTenantGetCommand(configPath, out))
	command.AddCommand(newTenantCreateCommand(configPath, out))
	command.AddCommand(newTenantApproveCommand(configPath, out))
	command.AddCommand(newTenantDenyCommand(configPath, out))
	command.AddCommand(newTenantResetPasswordCommand(configPath, out))
	command.AddCommand(newTenantDeleteCommand(configPath, out))
	command.AddCommand(newTenantBootstrapCommand(configPath, out))
	return command
}
