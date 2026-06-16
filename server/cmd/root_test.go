package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/csweichel/flink/server/api"
	"github.com/csweichel/flink/server/storage"
	"github.com/csweichel/flink/shared/banner"
	bolt "go.etcd.io/bbolt"
)

func runCommand(args ...string) (string, error) {
	var out bytes.Buffer
	command := NewRootCommandWithOptions(Options{Out: &out, Err: &out})
	command.SetArgs(args)
	err := command.Execute()
	return out.String(), err
}

func TestRunInitWritesDefaultYAMLConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flink.yaml")

	out, err := runCommand("--config", path, "init")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "wrote "+path) {
		t.Fatalf("unexpected output: %q", out)
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
		"dropTenantDomainPrefix: true",
		"disableTenantRegistration: false",
		"defaultSiteAuthMode: owner",
		"baseURL: https://api.openai.com/v1",
		"model: gpt-4.1-mini",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("config missing %q:\n%s", want, got)
		}
	}

	if _, err := runCommand("--config", path, "init"); err == nil {
		t.Fatal("expected init to refuse overwriting existing config")
	}
}

func TestHelpPrintsPlainBannerWhenCaptured(t *testing.T) {
	out, err := runCommand("--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "flink") || !strings.Contains(out, "publish • host • realtime") {
		t.Fatalf("help should include banner, got %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("captured help should not contain ANSI color: %q", out)
	}
}

func TestFlinkBannerColorRendering(t *testing.T) {
	out := banner.Render(true)
	if !strings.Contains(out, "\x1b[38;2;") || !strings.Contains(out, "flink") {
		t.Fatalf("color banner should contain ANSI and text, got %q", out)
	}
}

func TestLoadServerConfigUsesYAMLOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flink.yaml")
	config := `addr: :9000
dataDir: /from-file
storage: bbolt
baseHost: file.internal
autoApproveTenants: true
disableTenantRegistration: true
defaultSiteAuthMode: none
dropTenantDomainPrefix: false
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
	if !cfg.AutoApproveTenants {
		t.Fatal("expected autoApproveTenants from config")
	}
	if !cfg.DisableTenantRegistration {
		t.Fatal("expected disableTenantRegistration from config")
	}
	if cfg.DefaultSiteAuthMode != api.SiteAuthNone {
		t.Fatalf("unexpected default site auth mode: %q", cfg.DefaultSiteAuthMode)
	}
	if cfg.DropTenantDomainPrefix {
		t.Fatal("expected dropTenantDomainPrefix from config")
	}
	if cfg.AI.APIKey != "test-key" || cfg.AI.BaseURL != "http://ai.local/v1" || cfg.AI.Model != "test-model" {
		t.Fatalf("unexpected AI config: %#v", cfg.AI)
	}
	if len(cfg.BootstrapTenants) != 1 || cfg.BootstrapTenants[0].Username != "demo" || cfg.BootstrapTenants[0].Password != "secret" {
		t.Fatalf("unexpected bootstrap tenants: %#v", cfg.BootstrapTenants)
	}
}

func TestTenantCommandsUseConfigFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flink.yaml")
	if err := os.WriteFile(path, []byte("addr: :9000\ndataDir: /from-file\nstorage: bbolt\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != "/from-file" || cfg.StorageDriver != "bbolt" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if _, err := runCommand("tenants", "approve", "alice", "--storage", "file", "--config", path); err == nil {
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

	if _, err := runCommand("tenants", "bootstrap", "demo", "secret", "--config", configPath); err != nil {
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

func TestTenantStoreInitErrorExplainsBoltTimeout(t *testing.T) {
	cfg := defaultServerConfig()
	cfg.StorageDriver = "bbolt"

	err := tenantStoreInitError(cfg, bolt.ErrTimeout)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "stop any running flink server instance") {
		t.Fatalf("expected actionable bbolt timeout message, got %q", err.Error())
	}
	if !errors.Is(err, bolt.ErrTimeout) {
		t.Fatalf("expected wrapped bolt timeout, got %v", err)
	}
}

func TestTenantStoreInitErrorLeavesOtherBackendsAlone(t *testing.T) {
	cfg := defaultServerConfig()
	cfg.StorageDriver = "file"

	err := tenantStoreInitError(cfg, bolt.ErrTimeout)
	if !errors.Is(err, bolt.ErrTimeout) {
		t.Fatalf("expected original timeout, got %v", err)
	}
	if strings.Contains(err.Error(), "stop any running flink server instance") {
		t.Fatalf("did not expect bbolt guidance for file backend, got %q", err.Error())
	}
}

func TestRunTenantsManagementCommands(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "flink.yaml")
	dataDir := filepath.Join(dir, "data")
	if err := os.WriteFile(configPath, []byte("addr: :8080\ndataDir: "+dataDir+"\nstorage: file\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		args = append(args, "--config", configPath)
		if _, err := runCommand(append([]string{"tenants"}, args...)...); err != nil {
			t.Fatalf("runCommand(%v): %v", args, err)
		}
	}
	store := func() (*api.Store, storage.Backend) {
		t.Helper()
		backend, err := storage.Open("file", dataDir)
		if err != nil {
			t.Fatal(err)
		}
		if err := backend.Init(context.Background()); err != nil {
			t.Fatal(err)
		}
		return api.NewStore(backend, ""), backend
	}

	run("create", "demo", "secret")
	if _, err := runCommand("tenants", "create", "demo", "other-secret", "--config", configPath); err == nil {
		t.Fatal("create should reject an existing tenant")
	}
	s, backend := store()
	if _, err := s.AuthenticateTenant("demo", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSite("demo", "site", ""); err != nil {
		t.Fatal(err)
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}

	run("deny", "demo")
	s, backend = store()
	if _, err := s.AuthenticateTenant("demo", "secret"); err == nil {
		t.Fatal("denied tenant should not authenticate")
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}

	run("approve", "demo")
	run("reset-password", "demo", "new-secret")
	run("get", "demo")
	run("list", "approved")

	s, backend = store()
	if _, err := s.AuthenticateTenant("demo", "secret"); err == nil {
		t.Fatal("old password should not authenticate")
	}
	if _, err := s.AuthenticateTenant("demo", "new-secret"); err != nil {
		t.Fatal(err)
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}

	run("delete", "demo")
	s, backend = store()
	if _, err := s.ReadTenant("demo"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("expected deleted tenant to be gone, got %v", err)
	}
	if _, err := s.ReadMeta("demo", "site"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("expected tenant sites to be deleted, got %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
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
