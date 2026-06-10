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
	Addr          string `yaml:"addr"`
	DataDir       string `yaml:"dataDir"`
	StorageDriver string `yaml:"storage"`
	BaseHost      string `yaml:"baseHost"`
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
	cfg := defaultServerConfig()
	var configPath string
	var addrFlag string
	var dataFlag string
	var storageFlag string
	var baseHostFlag string
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	fs.StringVar(&configPath, "config", env("FLINK_CONFIG", ""), "optional YAML config file")
	fs.StringVar(&addrFlag, "addr", "", "listen address")
	fs.StringVar(&dataFlag, "data", "", "data directory")
	fs.StringVar(&storageFlag, "storage", "", "storage driver: file, bbolt, dynamodb, firebase")
	fs.StringVar(&baseHostFlag, "base-host", "", "optional wildcard host suffix, e.g. quick.internal")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := applyConfigFile(&cfg, configPath); err != nil {
		return err
	}
	applyEnv(&cfg)
	applyOverrides(&cfg, addrFlag, dataFlag, storageFlag, baseHostFlag)

	app := serverapp.New(serverapp.Config{
		DataDir:       cfg.DataDir,
		StorageDriver: cfg.StorageDriver,
		BaseHost:      cfg.BaseHost,
	})
	if err := app.Init(); err != nil {
		return err
	}
	defer app.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return serverapp.ListenAndServe(ctx, cfg.Addr, app)
}

func runTenants(args []string) error {
	args, cfg, err := parseTenantCommandArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: flink-server tenants <pending|approve|deny> [username]")
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
	case "pending", "list":
		status := api.TenantPending
		if args[0] == "list" {
			status = ""
		}
		tenants, err := store.ListTenants(status)
		if err != nil {
			return err
		}
		for _, tenant := range tenants {
			fmt.Printf("%-24s %-10s %s\n", tenant.Username, tenant.Status, tenant.UpdatedAt.Format(time.RFC3339))
		}
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
	default:
		return fmt.Errorf("unknown tenants command %q", args[0])
	}
	return nil
}

func runInit(args []string) error {
	cfg := defaultServerConfig()
	configPath := "flink.yaml"
	force := false
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fs.StringVar(&configPath, "config", configPath, "config file to write")
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.StringVar(&cfg.DataDir, "data", cfg.DataDir, "data directory")
	fs.StringVar(&cfg.StorageDriver, "storage", cfg.StorageDriver, "storage driver")
	fs.StringVar(&cfg.BaseHost, "base-host", cfg.BaseHost, "optional wildcard host suffix")
	fs.BoolVar(&force, "force", false, "overwrite an existing config file")
	if err := fs.Parse(args); err != nil {
		return err
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
	cfg := defaultServerConfig()
	var configPath string
	var dataFlag string
	var storageFlag string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--config":
			i++
			if i >= len(args) {
				return nil, cfg, fmt.Errorf("--config requires a value")
			}
			configPath = args[i]
		case strings.HasPrefix(arg, "--config="):
			configPath = strings.TrimPrefix(arg, "--config=")
		case arg == "--data":
			i++
			if i >= len(args) {
				return nil, cfg, fmt.Errorf("--data requires a value")
			}
			dataFlag = args[i]
		case strings.HasPrefix(arg, "--data="):
			dataFlag = strings.TrimPrefix(arg, "--data=")
		case arg == "--storage":
			i++
			if i >= len(args) {
				return nil, cfg, fmt.Errorf("--storage requires a value")
			}
			storageFlag = args[i]
		case strings.HasPrefix(arg, "--storage="):
			storageFlag = strings.TrimPrefix(arg, "--storage=")
		default:
			positional = append(positional, arg)
		}
	}
	if err := applyConfigFile(&cfg, configPath); err != nil {
		return nil, cfg, err
	}
	applyEnv(&cfg)
	applyOverrides(&cfg, "", dataFlag, storageFlag, "")
	return positional, cfg, nil
}

func defaultServerConfig() serverConfig {
	return serverConfig{
		Addr:          ":8080",
		DataDir:       "./data",
		StorageDriver: "file",
		BaseHost:      "",
	}
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
}

func applyEnv(cfg *serverConfig) {
	applyConfigValues(cfg, serverConfig{
		Addr:          os.Getenv("FLINK_ADDR"),
		DataDir:       os.Getenv("FLINK_DATA"),
		StorageDriver: os.Getenv("FLINK_STORAGE"),
		BaseHost:      os.Getenv("FLINK_BASE_HOST"),
	})
}

func applyOverrides(cfg *serverConfig, addr, dataDir, storageDriver, baseHost string) {
	applyConfigValues(cfg, serverConfig{
		Addr:          addr,
		DataDir:       dataDir,
		StorageDriver: storageDriver,
		BaseHost:      baseHost,
	})
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
