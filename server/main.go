package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"flink/server/api"
	serverapp "flink/server/app"
	"flink/server/storage"

	"gopkg.in/yaml.v3"
)

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

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		if err := runInit(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "tenants" {
		if err := runTenants(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := runServe(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func runServe(args []string) error {
	configPath, err := parseConfigFlag("serve", args)
	if err != nil {
		return err
	}
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return serverapp.ListenAndServe(ctx, cfg.Addr, app)
}

func parseConfigFlag(name string, args []string) (string, error) {
	var configPath string
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.StringVar(&configPath, "config", "", "YAML config file")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() != 0 {
		return "", fmt.Errorf("%s accepts only --config; put server settings in the YAML config file", name)
	}
	return configPath, nil
}

func runTenants(args []string) error {
	args, cfg, err := parseTenantCommandArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: flink-server tenants <list|pending|get|create|approve|deny|reset-password|delete|bootstrap> [args]")
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

	switch args[0] {
	case "pending":
		if len(args) != 1 {
			return fmt.Errorf("usage: flink-server tenants pending")
		}
		if err := printTenants(store, api.TenantPending); err != nil {
			return err
		}
	case "list":
		if len(args) > 2 {
			return fmt.Errorf("usage: flink-server tenants list [pending|approved|denied|all]")
		}
		status := ""
		if len(args) == 2 && args[1] != "all" {
			switch args[1] {
			case api.TenantPending, api.TenantApproved, api.TenantDenied:
				status = args[1]
			default:
				return fmt.Errorf("unknown tenant status %q", args[1])
			}
		}
		if err := printTenants(store, status); err != nil {
			return err
		}
	case "get":
		if len(args) != 2 {
			return fmt.Errorf("usage: flink-server tenants get <username>")
		}
		tenant, err := store.ReadTenant(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("%-24s %-10s created=%s updated=%s\n", tenant.Username, tenant.Status, tenant.CreatedAt.Format(time.RFC3339), tenant.UpdatedAt.Format(time.RFC3339))
	case "create":
		if len(args) != 3 {
			return fmt.Errorf("usage: flink-server tenants create <username> <password>")
		}
		tenant, err := store.RegisterApprovedTenant(args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Printf("created %s\n", tenant.Username)
	case "approve":
		if len(args) != 2 {
			return fmt.Errorf("usage: flink-server tenants approve <username>")
		}
		tenant, err := store.ApproveTenant(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("approved %s\n", tenant.Username)
	case "deny":
		if len(args) != 2 {
			return fmt.Errorf("usage: flink-server tenants deny <username>")
		}
		tenant, err := store.DenyTenant(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("denied %s\n", tenant.Username)
	case "reset-password":
		if len(args) != 3 {
			return fmt.Errorf("usage: flink-server tenants reset-password <username> <password>")
		}
		tenant, err := store.ResetTenantPassword(args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Printf("reset password for %s\n", tenant.Username)
	case "delete":
		if len(args) != 2 {
			return fmt.Errorf("usage: flink-server tenants delete <username>")
		}
		if err := store.DeleteTenant(args[1]); err != nil {
			return err
		}
		fmt.Printf("deleted %s\n", args[1])
	case "bootstrap":
		if len(args) != 3 {
			return fmt.Errorf("usage: flink-server tenants bootstrap <username> <password>")
		}
		tenant, err := store.CreateApprovedTenant(args[1], args[2])
		if err != nil {
			return err
		}
		fmt.Printf("bootstrapped %s\n", tenant.Username)
	default:
		return fmt.Errorf("unknown tenants command %q", args[0])
	}
	return nil
}

func printTenants(store *api.Store, status string) error {
	tenants, err := store.ListTenants(status)
	if err != nil {
		return err
	}
	for _, tenant := range tenants {
		fmt.Printf("%-24s %-10s %s\n", tenant.Username, tenant.Status, tenant.UpdatedAt.Format(time.RFC3339))
	}
	return nil
}

func runInit(args []string) error {
	cfg := defaultServerConfig()
	configPath := "flink.yaml"
	force := false
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fs.StringVar(&configPath, "config", configPath, "config file to write")
	fs.BoolVar(&force, "force", false, "overwrite an existing config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("init accepts only --config and --force; edit server settings in the YAML config file")
	}
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
	fmt.Printf("wrote %s\n", configPath)
	return nil
}

func parseTenantCommandArgs(args []string) ([]string, serverConfig, error) {
	var configPath string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--config":
			i++
			if i >= len(args) {
				return nil, serverConfig{}, fmt.Errorf("--config requires a value")
			}
			configPath = args[i]
		case strings.HasPrefix(arg, "--config="):
			configPath = strings.TrimPrefix(arg, "--config=")
		case strings.HasPrefix(arg, "--"):
			return nil, serverConfig{}, fmt.Errorf("unknown flag %s; put server settings in the YAML config file", arg)
		default:
			positional = append(positional, arg)
		}
	}
	cfg, err := loadServerConfig(configPath)
	if err != nil {
		return nil, cfg, err
	}
	return positional, cfg, nil
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
