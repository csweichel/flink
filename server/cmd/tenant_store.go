package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/csweichel/flink/server/api"
	"github.com/csweichel/flink/server/storage"
	bolt "go.etcd.io/bbolt"
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
		return tenantStoreInitError(cfg, err)
	}
	defer backend.Close()
	return fn(api.NewStore(backend, ""))
}

func tenantStoreInitError(cfg serverConfig, err error) error {
	if isBoltStorage(cfg.StorageDriver) && errors.Is(err, bolt.ErrTimeout) {
		return fmt.Errorf("%w: bbolt storage is locked; stop any running flink server instance before using this command right now", err)
	}
	return err
}

func isBoltStorage(driver string) bool {
	return driver == "bbolt" || driver == "bolt"
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
