package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const projectConfigPath = ".flink/site.json"

type commandContext struct {
	serverURL *string
	username  *string
	password  *string
	jsonOut   *bool
}

type flinkConfig struct {
	Site     string `json:"site,omitempty"`
	Server   string `json:"server,omitempty"`
	Tenant   string `json:"tenant,omitempty"`
	Password string `json:"password,omitempty"`
}

type resolvedConfig struct {
	Site     string
	Server   string
	Tenant   string
	Password string
}

func (ctx *commandContext) resolveConfig() resolvedConfig {
	user := readUserConfig()
	project := readProjectConfig("")
	serverFlag := strings.TrimSpace(*ctx.serverURL)
	if serverFlag == "http://localhost:8080" {
		serverFlag = ""
	}
	return resolvedConfig{
		Site:     firstNonEmpty(project.Site, user.Site),
		Server:   firstNonEmpty(serverFlag, env("FLINK_SERVER", ""), project.Server, user.Server, "http://localhost:8080"),
		Tenant:   firstNonEmpty(*ctx.username, envAny([]string{"FLINK_TENANT", "FLINK_USERNAME"}, ""), project.Tenant, user.Tenant),
		Password: firstNonEmpty(*ctx.password, env("FLINK_PASSWORD", ""), user.Password),
	}
}

func (ctx *commandContext) client() (*client, resolvedConfig, error) {
	config := ctx.resolveConfig()
	c, err := newClient(config.Server, config.Tenant, config.Password)
	return c, config, err
}

func (ctx *commandContext) writeJSON(cmd *cobra.Command, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(b))
	return err
}

func (ctx *commandContext) wantsJSON() bool {
	return ctx.jsonOut != nil && *ctx.jsonOut
}

func readProjectConfig(root string) flinkConfig {
	if root == "" {
		root = "."
	}
	var config flinkConfig
	b, err := os.ReadFile(filepath.Join(root, projectConfigPath))
	if err != nil {
		return config
	}
	_ = json.Unmarshal(b, &config)
	return config
}

func writeProjectConfig(root string, config flinkConfig) error {
	if root == "" {
		root = "."
	}
	path := filepath.Join(root, projectConfigPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}

func userConfigPath() string {
	if path := env("FLINK_CONFIG", ""); path != "" {
		return path
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, "flink", "config.json")
}

func readUserConfig() flinkConfig {
	var config flinkConfig
	path := userConfigPath()
	if path == "" {
		return config
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return config
	}
	_ = json.Unmarshal(b, &config)
	return config
}

func writeUserConfig(config flinkConfig) error {
	path := userConfigPath()
	if path == "" {
		return fmt.Errorf("cannot locate user config directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func canonicalSiteURL(server, tenant, slug string) string {
	u, err := url.Parse(server)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.TrimRight(server, "/") + "/t/" + tenant + "/s/" + slug + "/"
	}
	u.Path = strings.TrimRight(u.Path, "/")
	host := u.Hostname()
	if u.Port() == "" && u.EscapedPath() == "" && host != "localhost" {
		return u.Scheme + "://" + slug + "." + host + "/"
	}
	return strings.TrimRight(u.String(), "/") + "/t/" + tenant + "/s/" + slug + "/"
}
