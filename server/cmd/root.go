package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"flink/server/api"
	serverapp "flink/server/app"
	"flink/server/storage"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type Options struct {
	Out           io.Writer
	Err           io.Writer
	SignalContext func() (context.Context, context.CancelFunc)
}

type serverConfig struct {
	Addr               string                  `yaml:"addr"`
	DataDir            string                  `yaml:"dataDir"`
	StorageDriver      string                  `yaml:"storage"`
	BaseHost           string                  `yaml:"baseHost"`
	AutoApproveTenants bool                    `yaml:"autoApproveTenants"`
	AI                 api.AIConfig            `yaml:"ai"`
	BootstrapTenants   []bootstrapTenantConfig `yaml:"bootstrapTenants"`
}

type bootstrapTenantConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
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
	installBannerHelp(root)
	root.AddCommand(newInitCommand(&configPath, out))
	root.AddCommand(newTenantsCommand(&configPath, out))
	return root
}

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

func newTenantsCommand(configPath *string, out io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "tenants",
		Short: "Manage approved and pending tenants",
	}
	command.AddCommand(&cobra.Command{
		Use:   "list [pending|approved|denied|all]",
		Short: "List tenants",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			status := ""
			if len(args) == 1 && args[0] != "all" {
				switch args[0] {
				case api.TenantPending, api.TenantApproved, api.TenantDenied:
					status = args[0]
				default:
					return fmt.Errorf("unknown tenant status %q", args[0])
				}
			}
			return withTenantStore(*configPath, func(store *api.Store) error {
				return printTenants(out, store, status)
			})
		},
	})
	command.AddCommand(&cobra.Command{
		Use:   "pending",
		Short: "List pending tenant registrations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				return printTenants(out, store, api.TenantPending)
			})
		},
	})
	command.AddCommand(&cobra.Command{
		Use:   "get <username>",
		Short: "Show tenant details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				tenant, err := store.ReadTenant(args[0])
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "%-24s %-10s created=%s updated=%s\n", tenant.Username, tenant.Status, tenant.CreatedAt.Format(time.RFC3339), tenant.UpdatedAt.Format(time.RFC3339))
				return nil
			})
		},
	})
	command.AddCommand(&cobra.Command{
		Use:   "create <username> <password>",
		Short: "Create a new approved tenant",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				tenant, err := store.RegisterApprovedTenant(args[0], args[1])
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "created %s\n", tenant.Username)
				return nil
			})
		},
	})
	command.AddCommand(tenantStatusCommand("approve", "Approve a pending tenant", api.TenantApproved, configPath, out))
	command.AddCommand(tenantStatusCommand("deny", "Deny a tenant", api.TenantDenied, configPath, out))
	command.AddCommand(&cobra.Command{
		Use:   "reset-password <username> <password>",
		Short: "Reset a tenant password",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				tenant, err := store.ResetTenantPassword(args[0], args[1])
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "reset password for %s\n", tenant.Username)
				return nil
			})
		},
	})
	command.AddCommand(&cobra.Command{
		Use:   "delete <username>",
		Short: "Delete a tenant and all of its sites",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				if err := store.DeleteTenant(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(out, "deleted %s\n", args[0])
				return nil
			})
		},
	})
	command.AddCommand(&cobra.Command{
		Use:   "bootstrap <username> <password>",
		Short: "Create or update an approved tenant for automation",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				tenant, err := store.CreateApprovedTenant(args[0], args[1])
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "bootstrapped %s\n", tenant.Username)
				return nil
			})
		},
	})
	return command
}

func tenantStatusCommand(name, short, status string, configPath *string, out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <username>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withTenantStore(*configPath, func(store *api.Store) error {
				var (
					tenant api.PublicTenant
					err    error
				)
				switch status {
				case api.TenantApproved:
					tenant, err = store.ApproveTenant(args[0])
				case api.TenantDenied:
					tenant, err = store.DenyTenant(args[0])
				default:
					return fmt.Errorf("unsupported tenant status %q", status)
				}
				if err != nil {
					return err
				}
				verb := "approved"
				if status == api.TenantDenied {
					verb = "denied"
				}
				fmt.Fprintf(out, "%s %s\n", verb, tenant.Username)
				return nil
			})
		},
	}
}

func runServe(configPath string, options Options) error {
	cfg, err := loadServerConfig(configPath)
	if err != nil {
		return err
	}
	if err := bootstrapConfiguredTenants(cfg); err != nil {
		return err
	}

	app := serverapp.New(serverapp.Config{
		DataDir:            cfg.DataDir,
		StorageDriver:      cfg.StorageDriver,
		BaseHost:           cfg.BaseHost,
		AutoApproveTenants: cfg.AutoApproveTenants,
		AI:                 cfg.AI,
	})
	if err := app.Init(); err != nil {
		return err
	}
	defer app.Close()
	signalContext := options.SignalContext
	if signalContext == nil {
		signalContext = defaultSignalContext
	}
	ctx, stop := signalContext()
	defer stop()
	return serverapp.ListenAndServe(ctx, cfg.Addr, app)
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

func withTenantStore(configPath string, fn func(*api.Store) error) error {
	cfg, err := loadServerConfig(configPath)
	if err != nil {
		return err
	}
	backend, err := storage.Open(cfg.StorageDriver, cfg.DataDir)
	if err != nil {
		return err
	}
	if err := backend.Init(context.Background()); err != nil {
		return err
	}
	defer backend.Close()
	return fn(api.NewStore(backend, ""))
}

func printTenants(out io.Writer, store *api.Store, status string) error {
	tenants, err := store.ListTenants(status)
	if err != nil {
		return err
	}
	for _, tenant := range tenants {
		fmt.Fprintf(out, "%-24s %-10s %s\n", tenant.Username, tenant.Status, tenant.UpdatedAt.Format(time.RFC3339))
	}
	return nil
}

func defaultServerConfig() serverConfig {
	return serverConfig{
		Addr:               ":8080",
		DataDir:            "./data",
		StorageDriver:      "file",
		BaseHost:           "",
		AutoApproveTenants: false,
		AI: api.AIConfig{
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4.1-mini",
		},
	}
}

func loadServerConfig(configPath string) (serverConfig, error) {
	cfg := defaultServerConfig()
	if err := applyConfigFile(&cfg, configPath); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func applyConfigFile(cfg *serverConfig, configPath string) error {
	if configPath == "" {
		if _, err := os.Stat("flink.yaml"); err == nil {
			configPath = "flink.yaml"
		} else {
			return nil
		}
	}
	b, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var fileCfg serverConfig
	if err := yaml.Unmarshal(b, &fileCfg); err != nil {
		return err
	}
	applyConfigValues(cfg, fileCfg)
	return nil
}

func applyConfigValues(cfg *serverConfig, override serverConfig) {
	if override.Addr != "" {
		cfg.Addr = override.Addr
	}
	if override.DataDir != "" {
		cfg.DataDir = override.DataDir
	}
	if override.StorageDriver != "" {
		cfg.StorageDriver = override.StorageDriver
	}
	if override.BaseHost != "" {
		cfg.BaseHost = override.BaseHost
	}
	if override.AutoApproveTenants {
		cfg.AutoApproveTenants = true
	}
	if override.AI.APIKey != "" {
		cfg.AI.APIKey = override.AI.APIKey
	}
	if override.AI.BaseURL != "" {
		cfg.AI.BaseURL = override.AI.BaseURL
	}
	if override.AI.Model != "" {
		cfg.AI.Model = override.AI.Model
	}
	if override.BootstrapTenants != nil {
		cfg.BootstrapTenants = override.BootstrapTenants
	}
}

func bootstrapConfiguredTenants(cfg serverConfig) error {
	if len(cfg.BootstrapTenants) == 0 {
		return nil
	}
	backend, err := storage.Open(cfg.StorageDriver, cfg.DataDir)
	if err != nil {
		return err
	}
	if err := backend.Init(context.Background()); err != nil {
		return err
	}
	defer backend.Close()

	store := api.NewStore(backend, "")
	for _, tenant := range cfg.BootstrapTenants {
		if strings.TrimSpace(tenant.Username) == "" && strings.TrimSpace(tenant.Password) == "" {
			continue
		}
		if _, err := store.CreateApprovedTenant(tenant.Username, tenant.Password); err != nil {
			return err
		}
	}
	return nil
}

func defaultSignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}
