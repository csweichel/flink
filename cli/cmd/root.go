package cmd

import (
	"github.com/csweichel/flink/shared/banner"

	"github.com/spf13/cobra"
)

type Options struct {
	ServerURL string
	Tenant    string
	Password  string
}

func NewRootCommand() *cobra.Command {
	return NewRootCommandWithOptions(Options{
		ServerURL: env("FLINK_SERVER", "http://localhost:8080"),
		Tenant:    envAny([]string{"FLINK_TENANT", "FLINK_USERNAME"}, ""),
		Password:  env("FLINK_PASSWORD", ""),
	})
}

func NewRootCommandWithOptions(options Options) *cobra.Command {
	serverURL := options.ServerURL
	username := options.Tenant
	password := options.Password
	jsonOut := false

	root := &cobra.Command{
		Use:   "flink",
		Short: "User CLI for publishing and managing Flink sites",
	}
	root.PersistentFlags().StringVar(&serverURL, "server", serverURL, "Flink server URL")
	root.PersistentFlags().StringVar(&username, "tenant", username, "approved Flink tenant username")
	root.PersistentFlags().StringVar(&password, "password", password, "Flink tenant password")
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	banner.InstallHelp(root)

	ctx := &commandContext{
		serverURL: &serverURL,
		username:  &username,
		password:  &password,
		jsonOut:   &jsonOut,
	}
	root.AddCommand(publishCommand(ctx))
	root.AddCommand(initCommand(ctx))
	root.AddCommand(openCommand(ctx))
	root.AddCommand(listCommand(ctx))
	root.AddCommand(inspectCommand(ctx))
	root.AddCommand(historyCommand(ctx))
	root.AddCommand(rollbackCommand(ctx))
	root.AddCommand(snapshotCommand(ctx))
	root.AddCommand(authCommand(ctx))
	root.AddCommand(configCommand(ctx))

	return root
}
