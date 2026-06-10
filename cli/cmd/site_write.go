package cmd

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func siteWriteCommand(serverURL, username, password *string) *cobra.Command {
	return &cobra.Command{
		Use:   "write <slug> <local-file-or-dir> [site-path]",
		Short: "Publish a local file or directory to a site",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newClient(*serverURL, *username, *password)
			if err != nil {
				return err
			}
			localPath := args[1]
			info, err := os.Stat(localPath)
			if err != nil {
				return err
			}
			if len(args) == 3 {
				if strings.TrimSpace(args[2]) == "" {
					return fmt.Errorf("site path cannot be empty")
				}
			}
			if info.IsDir() {
				prefix := ""
				if len(args) == 3 {
					prefix = strings.Trim(strings.ReplaceAll(args[2], "\\", "/"), "/")
				}
				published := 0
				err := filepath.WalkDir(localPath, func(path string, entry os.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					if entry.IsDir() {
						return nil
					}
					rel, err := filepath.Rel(localPath, path)
					if err != nil {
						return err
					}
					target := filepath.ToSlash(rel)
					if prefix != "" {
						target = prefix + "/" + target
					}
					if err := publishFile(c, args[0], path, target); err != nil {
						return err
					}
					published++
					return nil
				})
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "published %d files to %s\n", published, c.siteURL(args[0]))
				return nil
			}

			target := filepath.Base(localPath)
			if len(args) == 3 {
				target = args[2]
			}
			if err := publishFile(c, args[0], localPath, target); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "published %s to %s\n", filepath.ToSlash(target), c.siteURL(args[0]))
			return nil
		},
	}
}

func publishFile(c *client, slug, localPath, sitePath string) error {
	b, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	contentType := mime.TypeByExtension(filepath.Ext(localPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	path := fmt.Sprintf("/api/sites/%s/files?path=%s", url.PathEscape(slug), url.QueryEscape(filepath.ToSlash(sitePath)))
	var out map[string]string
	return c.doBytes(http.MethodPut, path, b, contentType, &out)
}
