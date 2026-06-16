package cmd

import (
	"os"

	"github.com/csweichel/flink/server/api"

	"gopkg.in/yaml.v3"
)

type serverConfig struct {
	Addr                      string                  `yaml:"addr"`
	DataDir                   string                  `yaml:"dataDir"`
	StorageDriver             string                  `yaml:"storage"`
	BaseHost                  string                  `yaml:"baseHost"`
	DropTenantDomainPrefix    bool                    `yaml:"dropTenantDomainPrefix"`
	DropTenantDomainPrefixSet bool                    `yaml:"-"`
	AutoApproveTenants        bool                    `yaml:"autoApproveTenants"`
	DisableTenantRegistration bool                    `yaml:"disableTenantRegistration"`
	DefaultSiteAuthMode       string                  `yaml:"defaultSiteAuthMode"`
	AI                        api.AIConfig            `yaml:"ai"`
	BootstrapTenants          []bootstrapTenantConfig `yaml:"bootstrapTenants"`
}

type bootstrapTenantConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

func defaultServerConfig() serverConfig {
	return serverConfig{
		Addr:                      ":8080",
		DataDir:                   "./data",
		StorageDriver:             "file",
		BaseHost:                  "",
		DropTenantDomainPrefix:    true,
		DropTenantDomainPrefixSet: true,
		AutoApproveTenants:        false,
		DisableTenantRegistration: false,
		DefaultSiteAuthMode:       api.SiteAuthOwner,
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
	var raw map[string]any
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return err
	}
	if _, ok := raw["dropTenantDomainPrefix"]; ok {
		cfg.DropTenantDomainPrefix = fileCfg.DropTenantDomainPrefix
		cfg.DropTenantDomainPrefixSet = true
	}
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
	if override.DropTenantDomainPrefixSet {
		cfg.DropTenantDomainPrefix = override.DropTenantDomainPrefix
		cfg.DropTenantDomainPrefixSet = true
	}
	if override.AutoApproveTenants {
		cfg.AutoApproveTenants = true
	}
	if override.DisableTenantRegistration {
		cfg.DisableTenantRegistration = true
	}
	if override.DefaultSiteAuthMode != "" {
		cfg.DefaultSiteAuthMode = override.DefaultSiteAuthMode
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
