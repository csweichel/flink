package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

type siteMeta struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Options struct {
	ServerURL string
	Tenant    string
	Password  string
}

func NewRootCommand() *cobra.Command {
	return NewRootCommandWithOptions(Options{
		ServerURL: env("FLINK_SERVER", "http://localhost:8080"),
		Tenant:    envAny([]string{"FLINK_TENANT", "FLINK_USERNAME"}, ""),
		Password:  env("FLINK_PASSWORD", ""),
	})
}

func NewRootCommandWithOptions(options Options) *cobra.Command {
	serverURL := options.ServerURL
	username := options.Tenant
	password := options.Password

	root := &cobra.Command{
		Use:   "flink",
		Short: "User CLI for publishing and managing Flink sites",
	}
	root.PersistentFlags().StringVar(&serverURL, "server", serverURL, "Flink server URL")
	root.PersistentFlags().StringVar(&username, "tenant", username, "approved Flink tenant username")
	root.PersistentFlags().StringVar(&password, "password", password, "Flink tenant password")

	site := &cobra.Command{Use: "site", Short: "Manage your sites on a Flink server"}
	site.AddCommand(siteCreateCommand(&serverURL, &username, &password))
	site.AddCommand(siteListCommand(&serverURL, &username, &password))
	site.AddCommand(siteWriteCommand(&serverURL, &username, &password))
	site.AddCommand(siteDeleteCommand(&serverURL, &username, &password))
	root.AddCommand(site)

	return root
}

func siteCreateCommand(serverURL, username, password *string) *cobra.Command {
	return &cobra.Command{
		Use:   "create <slug>",
		Short: "Create a site on the Flink server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(*serverURL, *username, *password)
			if err != nil {
				return err
			}
			var meta siteMeta
			if err := c.doJSON(http.MethodPost, "/api/sites", map[string]string{"slug": args[0]}, &meta); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created %s at %s\n", meta.Slug, c.siteURL(meta.Slug))
			return nil
		},
	}
}

func siteListCommand(serverURL, username, password *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sites on the Flink server",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(*serverURL, *username, *password)
			if err != nil {
				return err
			}
			var sites []siteMeta
			if err := c.doJSON(http.MethodGet, "/api/sites", nil, &sites); err != nil {
				return err
			}
			for _, s := range sites {
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-28s %s\n", s.Slug, s.UpdatedAt.Format(time.RFC3339), c.siteURL(s.Slug))
			}
			return nil
		},
	}
}

func siteWriteCommand(serverURL, username, password *string) *cobra.Command {
	return &cobra.Command{
		Use:   "write <slug> <local-file> [site-path]",
		Short: "Publish a local file to a site",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(*serverURL, *username, *password)
			if err != nil {
				return err
			}
			target := filepath.Base(args[1])
			if len(args) == 3 {
				target = args[2]
			}
			b, err := os.ReadFile(args[1])
			if err != nil {
				return err
			}
			path := fmt.Sprintf("/api/sites/%s/files?path=%s", url.PathEscape(args[0]), url.QueryEscape(target))
			var out map[string]string
			if err := c.doJSON(http.MethodPut, path, map[string]string{"content": string(b)}, &out); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "published %s to %s\n", out["path"], c.siteURL(args[0]))
			return nil
		},
	}
}

func siteDeleteCommand(serverURL, username, password *string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <slug>",
		Short: "Delete a site from the Flink server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(*serverURL, *username, *password)
			if err != nil {
				return err
			}
			var out map[string]bool
			if err := c.doJSON(http.MethodDelete, "/api/sites/"+url.PathEscape(args[0]), nil, &out); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", args[0])
			return nil
		},
	}
}

func newClient(rawURL, username, password string) (*client, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("missing server URL")
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return nil, fmt.Errorf("missing tenant username; pass --tenant or set FLINK_TENANT")
	}
	if strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("missing tenant password; pass --password or set FLINK_PASSWORD")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("server must be an absolute URL, got %q", rawURL)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return &client{
		baseURL:  strings.TrimRight(u.String(), "/"),
		username: username,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *client) doJSON(method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.SetBasicAuth(c.username, c.password)
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(b, &e) == nil && e.Error != "" {
			return fmt.Errorf("%s", e.Error)
		}
		return fmt.Errorf("request failed: %s", res.Status)
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

func (c *client) siteURL(slug string) string {
	return c.baseURL + "/t/" + c.username + "/s/" + slug + "/"
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envAny(keys []string, fallback string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return fallback
}
