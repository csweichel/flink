package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func snapshotCommand(ctx *commandContext) *cobra.Command {
	var zipOut bool
	cmd := &cobra.Command{
		Use:   "snapshot <site> [path]",
		Short: "Export hosted files as a static snapshot",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, config, err := ctx.client()
			if err != nil {
				return err
			}
			site := args[0]
			target := site + "-snapshot"
			if len(args) == 2 {
				target = args[1]
			}
			var files []siteFileInfo
			if err := c.doJSON(http.MethodGet, "/api/sites/"+url.PathEscape(site)+"/files", nil, &files); err != nil {
				return err
			}
			contents := map[string][]byte{}
			for _, file := range files {
				var out struct {
					Path    string `json:"path"`
					Content string `json:"content"`
				}
				path := "/api/sites/" + url.PathEscape(site) + "/files?path=" + url.QueryEscape(file.Path)
				if err := c.doJSON(http.MethodGet, path, nil, &out); err != nil {
					return err
				}
				contents[file.Path] = []byte(out.Content)
			}
			manifest := map[string]any{
				"site":      site,
				"tenant":    config.Tenant,
				"sourceURL": canonicalSiteURL(config.Server, config.Tenant, site),
				"createdAt": time.Now().UTC(),
				"files":     files,
			}
			manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
			if err != nil {
				return err
			}
			contents["flink-snapshot.json"] = append(manifestBytes, '\n')
			if zipOut || strings.HasSuffix(target, ".zip") {
				if err := writeSnapshotZip(target, contents); err != nil {
					return err
				}
			} else {
				if err := writeSnapshotDir(target, contents); err != nil {
					return err
				}
			}
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, map[string]any{"site": site, "path": target, "files": len(files)})
			}
			printSections(cmd.OutOrStdout(), "Snapshot written",
				outputSection{Title: "Source", Rows: []outputRow{
					row("Site", site),
					row("Tenant", config.Tenant),
					row("URL", canonicalSiteURL(config.Server, config.Tenant, site)),
				}},
				outputSection{Title: "Output", Rows: []outputRow{
					row("Path", target),
					row("Files", len(files)),
				}},
			)
			return nil
		},
	}
	cmd.Flags().BoolVar(&zipOut, "zip", false, "write a zip archive")
	return cmd
}

func writeSnapshotDir(root string, contents map[string][]byte) error {
	for name, b := range contents {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			return err
		}
	}
	return nil
}

func writeSnapshotZip(path string, contents map[string][]byte) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, b := range contents {
		w, err := zw.Create(filepath.ToSlash(name))
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, bytes.NewReader(b)); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}
