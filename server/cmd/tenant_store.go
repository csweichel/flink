package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/csweichel/flink/server/api"
	"github.com/csweichel/flink/server/storage"
)

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
