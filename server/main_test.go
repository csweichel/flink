package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInitWritesDefaultYAMLConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flink.yaml")

	if err := runInit([]string{"--config", path, "--addr", ":9090", "--data", "/var/lib/flink", "--storage", "bbolt", "--base-host", "flink.internal"}); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		"addr: :9090",
		"dataDir: /var/lib/flink",
		"storage: bbolt",
		"baseHost: flink.internal",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("config missing %q:\n%s", want, got)
		}
	}

	if err := runInit([]string{"--config", path}); err == nil {
		t.Fatal("expected init to refuse overwriting existing config")
	}
}

func TestApplyConfigFileEnvAndFlagsPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flink.yaml")
	if err := os.WriteFile(path, []byte("addr: :9000\ndataDir: /from-file\nstorage: bbolt\nbaseHost: file.internal\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLINK_DATA", "/from-env")

	cfg := defaultServerConfig()
	if err := applyConfigFile(&cfg, path); err != nil {
		t.Fatal(err)
	}
	applyEnv(&cfg)
	applyOverrides(&cfg, ":9999", "/from-flag", "", "flag.internal")

	if cfg.Addr != ":9999" || cfg.DataDir != "/from-flag" || cfg.StorageDriver != "bbolt" || cfg.BaseHost != "flag.internal" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestParseTenantCommandArgsUsesConfigAndFlagOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flink.yaml")
	if err := os.WriteFile(path, []byte("addr: :9000\ndataDir: /from-file\nstorage: file\n"), 0644); err != nil {
		t.Fatal(err)
	}

	args, cfg, err := parseTenantCommandArgs([]string{"approve", "alice", "--config", path, "--storage", "bbolt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != "approve" || args[1] != "alice" {
		t.Fatalf("unexpected positional args: %#v", args)
	}
	if cfg.DataDir != "/from-file" || cfg.StorageDriver != "bbolt" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}
