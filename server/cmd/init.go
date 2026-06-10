package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newInitCommand(configPath *string, out io.Writer) *cobra.Command {
	force := false
	command := &cobra.Command{
		Use:   "init",
		Short: "Write a default server YAML config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := strings.TrimSpace(*configPath)
			if path == "" {
				path = "flink.yaml"
			}
			return runInit(path, force, out)
		},
	}
	command.Flags().BoolVar(&force, "force", false, "overwrite an existing config file")
	return command
}

func runInit(configPath string, force bool, out io.Writer) error {
	cfg := defaultServerConfig()
	if !force {
		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("%s already exists; pass --force to overwrite", configPath)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	content := "# Flink server configuration\n" + string(b)
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s\n", configPath)
	return nil
}
