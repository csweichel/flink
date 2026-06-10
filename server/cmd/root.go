package cmd

import (
	"context"
	"io"
	"os"

	"github.com/csweichel/flink/shared/banner"

	"github.com/spf13/cobra"
)

type Options struct {
	Out           io.Writer
	Err           io.Writer
	SignalContext func() (context.Context, context.CancelFunc)
}

func NewRootCommand() *cobra.Command {
	return NewRootCommandWithOptions(Options{
		Out:           os.Stdout,
		Err:           os.Stderr,
		SignalContext: defaultSignalContext,
	})
}

func NewRootCommandWithOptions(options Options) *cobra.Command {
	out := options.Out
	if out == nil {
		out = io.Discard
	}
	errOut := options.Err
	if errOut == nil {
		errOut = io.Discard
	}
	var configPath string

	root := &cobra.Command{
		Use:           "flink-server",
		Short:         "Run and operate a Flink server",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(configPath, options)
		},
	}
	root.SetOut(out)
	root.SetErr(errOut)
	root.PersistentFlags().StringVar(&configPath, "config", "", "YAML config file")
	banner.InstallHelp(root)
	root.AddCommand(newInitCommand(&configPath, out))
	root.AddCommand(newTenantsCommand(&configPath, out))
	return root
}
