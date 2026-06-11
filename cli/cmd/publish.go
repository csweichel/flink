package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type localPublishFile struct {
	LocalPath string
	SitePath  string
	Size      int
	Hash      string
	Content   []byte
}

type publishResult struct {
	Site       string    `json:"site"`
	URL        string    `json:"url"`
	Created    bool      `json:"created"`
	Published  int       `json:"published"`
	Deleted    int       `json:"deleted"`
	Auth       string    `json:"auth"`
	Version    string    `json:"version,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt"`
	TotalBytes int       `json:"totalBytes"`
}

func publishCommand(ctx *commandContext) *cobra.Command {
	var site string
	var title string
	var owner bool
	var public bool
	var tenants []string
	cmd := &cobra.Command{
		Use:   "publish [path]",
		Short: "Create or update a site from local files",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			localPath := "."
			if len(args) == 1 {
				localPath = args[0]
			}
			c, config, err := ctx.client()
			if err != nil {
				return err
			}
			slug, err := inferSiteSlug(site, config.Site, localPath)
			if err != nil {
				return err
			}
			files, err := collectPublishFiles(localPath)
			if err != nil {
				return err
			}
			policy, setPolicy, err := selectedAuthPolicy(owner, public, tenants)
			if err != nil {
				return err
			}

			created := false
			var before siteMeta
			if err := c.doJSON(http.MethodGet, "/api/sites/"+url.PathEscape(slug), nil, &before); err != nil {
				created = true
			}
			var meta siteMeta
			if err := c.doJSON(http.MethodPost, "/api/sites", map[string]string{"slug": slug, "title": title}, &meta); err != nil {
				return err
			}
			if setPolicy {
				if err := c.doJSON(http.MethodPut, "/api/sites/"+url.PathEscape(slug)+"/auth", policy, &policy); err != nil {
					return err
				}
				meta.Auth = policy
			}
			deleted, err := deleteStaleFiles(c, slug, files)
			if err != nil {
				return err
			}
			for _, file := range files {
				if err := publishBytes(c, slug, file.SitePath, file.Content, mime.TypeByExtension(filepath.Ext(file.LocalPath))); err != nil {
					return err
				}
			}
			record := publishRecord{
				Source:       filepath.Base(cleanLocalPath(localPath)),
				GitCommit:    gitCommit(),
				FileCount:    len(files),
				TotalBytes:   totalPublishBytes(files),
				Files:        publishManifest(files),
				Auth:         meta.Auth,
				Capabilities: inferCapabilities(files, meta.Auth),
			}
			_ = c.doJSON(http.MethodPost, "/api/sites/"+url.PathEscape(slug)+"/publishes", record, &record)
			result := publishResult{
				Site:       slug,
				URL:        canonicalSiteURL(config.Server, config.Tenant, slug),
				Created:    created,
				Published:  len(files),
				Deleted:    deleted,
				Auth:       formatSiteAuthPolicy(meta.Auth),
				Version:    record.ID,
				UpdatedAt:  time.Now().UTC(),
				TotalBytes: record.TotalBytes,
			}
			_ = writeProjectConfig("", flinkConfig{Site: slug, Server: config.Server, Tenant: config.Tenant})
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, result)
			}
			action := "Publish complete"
			if result.Created {
				action = "Site created and published"
			}
			printSections(cmd.OutOrStdout(), action,
				outputSection{Title: "Target", Rows: []outputRow{
					row("Site", result.Site),
					row("Tenant", config.Tenant),
					row("Server", config.Server),
				}},
				outputSection{Title: "Result", Rows: append([]outputRow{
					row("Uploaded", fmt.Sprintf("%d files", result.Published)),
					row("Removed", fmt.Sprintf("%d stale files", result.Deleted)),
					row("Total size", formatBytes(result.TotalBytes)),
				}, optionalRow("Version", result.Version)...,
				)},
				outputSection{Title: "Access", Rows: []outputRow{
					row("Mode", result.Auth),
				}},
				outputSection{Title: "Links", Rows: []outputRow{
					row("Site", result.URL),
					row("Dashboard", strings.TrimRight(config.Server, "/")+"/_flink"),
				}},
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "site slug")
	cmd.Flags().StringVar(&title, "title", "", "site title")
	cmd.Flags().BoolVar(&owner, "owner", false, "restrict site to the owner tenant")
	cmd.Flags().BoolVar(&public, "public", false, "allow anonymous site access")
	cmd.Flags().StringSliceVar(&tenants, "tenants", nil, "allow signed-in tenants, optionally limited to this comma-separated list")
	return cmd
}

func inferSiteSlug(explicit, configured, localPath string) (string, error) {
	slug := firstNonEmpty(explicit, configured)
	if slug == "" {
		slug = filepath.Base(cleanLocalPath(localPath))
	}
	slug = slugify(slug)
	if slug == "" {
		return "", fmt.Errorf("could not infer site slug; pass --site")
	}
	return slug, nil
}

var nonSlugChar = regexp.MustCompile(`[^a-z0-9-]+`)

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	value = nonSlugChar.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	if len(value) > 63 {
		value = strings.Trim(value[:63], "-")
	}
	return value
}

func cleanLocalPath(path string) string {
	clean, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return clean
}

func collectPublishFiles(localPath string) ([]localPublishFile, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		target := filepath.Base(localPath)
		if strings.EqualFold(filepath.Ext(target), ".html") {
			target = "index.html"
		}
		file, err := readPublishFile(localPath, target)
		if err != nil {
			return nil, err
		}
		return []localPublishFile{file}, nil
	}
	var files []localPublishFile
	err = filepath.WalkDir(localPath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != localPath && ignoreDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if ignoreFile(entry.Name()) {
			return nil
		}
		rel, err := filepath.Rel(localPath, path)
		if err != nil {
			return err
		}
		file, err := readPublishFile(path, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].SitePath < files[j].SitePath })
	if len(files) == 0 {
		return nil, fmt.Errorf("no publishable files found in %s", localPath)
	}
	return files, nil
}

func readPublishFile(localPath, sitePath string) (localPublishFile, error) {
	b, err := os.ReadFile(localPath)
	if err != nil {
		return localPublishFile{}, err
	}
	sum := sha256.Sum256(b)
	return localPublishFile{
		LocalPath: localPath,
		SitePath:  filepath.ToSlash(sitePath),
		Size:      len(b),
		Hash:      "sha256:" + hex.EncodeToString(sum[:]),
		Content:   b,
	}, nil
}

func ignoreDir(name string) bool {
	switch name {
	case ".git", ".flink", "node_modules":
		return true
	default:
		return false
	}
}

func ignoreFile(name string) bool {
	switch name {
	case ".DS_Store", "Thumbs.db":
		return true
	default:
		return false
	}
}

func selectedAuthPolicy(owner, public bool, tenants []string) (siteAuthPolicy, bool, error) {
	selected := 0
	if owner {
		selected++
	}
	if public {
		selected++
	}
	if tenants != nil {
		selected++
	}
	if selected > 1 {
		return siteAuthPolicy{}, false, fmt.Errorf("choose only one of --owner, --public, or --tenants")
	}
	switch {
	case owner:
		return siteAuthPolicy{Mode: "owner"}, true, nil
	case public:
		return siteAuthPolicy{Mode: "none"}, true, nil
	case tenants != nil:
		return siteAuthPolicy{Mode: "tenants", Tenants: tenants}, true, nil
	default:
		return siteAuthPolicy{}, false, nil
	}
}

func deleteStaleFiles(c *client, slug string, files []localPublishFile) (int, error) {
	var existing []siteFileInfo
	if err := c.doJSON(http.MethodGet, "/api/sites/"+url.PathEscape(slug)+"/files", nil, &existing); err != nil {
		return 0, nil
	}
	next := map[string]bool{}
	for _, file := range files {
		next[file.SitePath] = true
	}
	deleted := 0
	for _, file := range existing {
		if next[file.Path] {
			continue
		}
		path := "/api/sites/" + url.PathEscape(slug) + "/files?path=" + url.QueryEscape(file.Path)
		var out map[string]bool
		if err := c.doJSON(http.MethodDelete, path, nil, &out); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func publishBytes(c *client, slug, sitePath string, b []byte, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	path := fmt.Sprintf("/api/sites/%s/files?path=%s", url.PathEscape(slug), url.QueryEscape(filepath.ToSlash(sitePath)))
	var out map[string]string
	return c.doBytes(http.MethodPut, path, b, contentType, &out)
}

func publishManifest(files []localPublishFile) []publishFileInfo {
	out := make([]publishFileInfo, 0, len(files))
	for _, file := range files {
		out = append(out, publishFileInfo{Path: file.SitePath, Size: file.Size, Hash: file.Hash})
	}
	return out
}

func totalPublishBytes(files []localPublishFile) int {
	total := 0
	for _, file := range files {
		total += file.Size
	}
	return total
}

func inferCapabilities(files []localPublishFile, auth siteAuthPolicy) []string {
	set := map[string]bool{"files": len(files) > 0}
	switch auth.Mode {
	case "none":
		set["public"] = true
	case "tenants":
		set["tenant-restricted"] = true
	default:
		set["owner-only"] = true
	}
	for _, file := range files {
		text := strings.ToLower(string(file.Content))
		if strings.Contains(text, ".storage") || strings.Contains(text, ".get(") || strings.Contains(text, ".set(") {
			set["storage"] = true
		}
		if strings.Contains(text, ".upload") || strings.Contains(text, ".uploads") {
			set["uploads"] = true
		}
		if strings.Contains(text, ".realtime") || strings.Contains(text, ".room(") || strings.Contains(text, "websocket") {
			set["realtime"] = true
		}
		if strings.Contains(text, ".ai(") || strings.Contains(text, ".ai.") {
			set["ai"] = true
		}
	}
	var out []string
	for capability, ok := range set {
		if ok {
			out = append(out, capability)
		}
	}
	sort.Strings(out)
	return out
}

func gitCommit() string {
	return ""
}
