package cmd

import "github.com/spf13/cobra"

func siteCommand(serverURL, username, password *string) *cobra.Command {
	site := &cobra.Command{Use: "site", Short: "Manage your sites on a Flink server"}
	site.AddCommand(siteCreateCommand(serverURL, username, password))
	site.AddCommand(siteListCommand(serverURL, username, password))
	site.AddCommand(siteWriteCommand(serverURL, username, password))
	site.AddCommand(siteFilesCommand(serverURL, username, password))
	site.AddCommand(siteDeleteFileCommand(serverURL, username, password))
	site.AddCommand(siteDeleteCommand(serverURL, username, password))
	return site
}
