package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

func openCommand(ctx *commandContext) *cobra.Command {
	var printOnly bool
	cmd := &cobra.Command{
		Use:   "open [site]",
		Short: "Open a site URL",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			config := ctx.resolveConfig()
			site := config.Site
			if len(args) == 1 {
				site = args[0]
			}
			if site == "" {
				return fmt.Errorf("missing site; pass a site or run inside a project with .flink/site.json")
			}
			url := canonicalSiteURL(config.Server, config.Tenant, site)
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, map[string]string{"site": site, "url": url})
			}
			fmt.Fprintln(cmd.OutOrStdout(), url)
			if printOnly {
				return nil
			}
			return openURL(url)
		},
	}
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the URL without opening a browser")
	return cmd
}

func openURL(rawURL string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command = "open"
		args = []string{rawURL}
	case "windows":
		command = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		command = "xdg-open"
		args = []string{rawURL}
	}
	if _, err := exec.LookPath(command); err != nil {
		return nil
	}
	return exec.Command(command, args...).Start()
}
