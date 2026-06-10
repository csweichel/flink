package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flink/server/api"
	"flink/server/storage"
)

func TestRunInitWritesDefaultYAMLConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flink.yaml")

	if err := runInit([]string{"--config", path}); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		"addr: :8080",
		"dataDir: ./data",
		"storage: file",
		"baseURL: https://api.openai.com/v1",
		"model: gpt-4.1-mini",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("config missing %q:\n%s", want, got)
		}
	}

	if err := runInit([]string{"--config", path}); err == nil {
		t.Fatal("expected init to refuse overwriting existing config")
	}
}

func TestLoadServerConfigUsesYAMLOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flink.yaml")
	config := `addr: :9000
dataDir: /from-file
storage: bbolt
baseHost: file.internal
ai:
  apiKey: test-key
  baseURL: http://ai.local/v1
  model: test-model
bootstrapTenants:
  - username: demo
    password: secret
`
	if err := os.WriteFile(path, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Addr != ":9000" || cfg.DataDir != "/from-file" || cfg.StorageDriver != "bbolt" || cfg.BaseHost != "file.internal" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.AI.APIKey != "test-key" || cfg.AI.BaseURL != "http://ai.local/v1" || cfg.AI.Model != "test-model" {
		t.Fatalf("unexpected AI config: %#v", cfg.AI)
	}
	if len(cfg.BootstrapTenants) != 1 || cfg.BootstrapTenants[0].Username != "demo" || cfg.BootstrapTenants[0].Password != "secret" {
		t.Fatalf("unexpected bootstrap tenants: %#v", cfg.BootstrapTenants)
	}
}

func TestParseTenantCommandArgsUsesConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flink.yaml")
	if err := os.WriteFile(path, []byte("addr: :9000\ndataDir: /from-file\nstorage: bbolt\n"), 0644); err != nil {
		t.Fatal(err)
	}

	args, cfg, err := parseTenantCommandArgs([]string{"approve", "alice", "--config", path})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != "approve" || args[1] != "alice" {
		t.Fatalf("unexpected positional args: %#v", args)
	}
	if cfg.DataDir != "/from-file" || cfg.StorageDriver != "bbolt" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if _, _, err := parseTenantCommandArgs([]string{"approve", "alice", "--storage", "file", "--config", path}); err == nil {
		t.Fatal("expected server setting flag to be rejected")
	}
}

func TestRunTenantsBootstrapCreatesApprovedTenant(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "flink.yaml")
	dataDir := filepath.Join(dir, "data")
	if err := os.WriteFile(configPath, []byte("addr: :8080\ndataDir: "+dataDir+"\nstorage: file\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runTenants([]string{"bootstrap", "demo", "secret", "--config", configPath}); err != nil {
		t.Fatal(err)
	}

	backend, err := storage.Open("file", dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	store := api.NewStore(backend, "")
	tenant, err := store.AuthenticateTenant("demo", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if tenant.Username != "demo" || tenant.Status != api.TenantApproved {
		t.Fatalf("unexpected tenant: %#v", tenant)
	}
}

func TestBootstrapConfiguredTenantsCreatesApprovedTenants(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultServerConfig()
	cfg.DataDir = dir
	cfg.BootstrapTenants = []bootstrapTenantConfig{{Username: "demo", Password: "secret"}}

	if err := bootstrapConfiguredTenants(cfg); err != nil {
		t.Fatal(err)
	}

	backend, err := storage.Open("file", dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	store := api.NewStore(backend, "")
	if _, err := store.AuthenticateTenant("demo", "secret"); err != nil {
		t.Fatal(err)
	}
}
