package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func apiCommand(ctx *commandContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Use Flink site APIs from the CLI",
		Long:  "Use the same storage, files, uploads, and AI APIs exposed to hosted sites through /flink.js.",
	}
	cmd.AddCommand(apiDataCommand(ctx))
	cmd.AddCommand(apiFilesCommand(ctx))
	cmd.AddCommand(apiUploadsCommand(ctx))
	cmd.AddCommand(apiAICommand(ctx))
	return cmd
}

func apiDataCommand(ctx *commandContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: "Read and write site JSON state",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "all <site>",
		Short: "Read all JSON state for a site",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			var out any
			if err := c.doJSON(http.MethodGet, siteAPIPath(args[0], "data/"), nil, &out); err != nil {
				return err
			}
			return ctx.writeJSON(cmd, out)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get <site> <key>",
		Short: "Read one JSON state key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			var out any
			if err := c.doJSON(http.MethodGet, siteAPIPath(args[0], "data/"+url.PathEscape(args[1])), nil, &out); err != nil {
				return err
			}
			return ctx.writeJSON(cmd, out)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "set <site> <key> <json|@file|->",
		Short: "Write one JSON state key",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			value, err := readJSONArgument(args[2], cmd.InOrStdin())
			if err != nil {
				return err
			}
			var out any
			if err := c.doJSON(http.MethodPut, siteAPIPath(args[0], "data/"+url.PathEscape(args[1])), value, &out); err != nil {
				return err
			}
			return ctx.writeJSON(cmd, out)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:     "delete <site> <key>",
		Aliases: []string{"del", "rm"},
		Short:   "Delete one JSON state key",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			var out map[string]bool
			if err := c.doJSON(http.MethodDelete, siteAPIPath(args[0], "data/"+url.PathEscape(args[1])), nil, &out); err != nil {
				return err
			}
			return ctx.writeJSON(cmd, out)
		},
	})
	return cmd
}

func apiFilesCommand(ctx *commandContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "files",
		Short: "Read and write hosted site files",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list <site> [prefix]",
		Short: "List hosted files",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			path := siteAPIPath(args[0], "files")
			if len(args) == 2 && args[1] != "" {
				path += "?prefix=" + url.QueryEscape(args[1])
			}
			var out []siteFileInfo
			if err := c.doJSON(http.MethodGet, path, nil, &out); err != nil {
				return err
			}
			return ctx.writeJSON(cmd, out)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "read <site> [path]",
		Short: "Read a hosted file",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			p := "index.html"
			if len(args) == 2 {
				p = args[1]
			}
			var out siteFile
			if err := c.doJSON(http.MethodGet, siteAPIPath(args[0], "files")+"?path="+url.QueryEscape(p), nil, &out); err != nil {
				return err
			}
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, out)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), out.Content)
			return err
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "write <site> <path> <file|->",
		Short: "Write a hosted file from a local file or stdin",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			b, err := readBytesArgument(args[2], cmd.InOrStdin())
			if err != nil {
				return err
			}
			var out map[string]string
			if err := c.doBytes(http.MethodPut, siteAPIPath(args[0], "files")+"?path="+url.QueryEscape(args[1]), b, "", &out); err != nil {
				return err
			}
			return ctx.writeJSON(cmd, out)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:     "delete <site> <path>",
		Aliases: []string{"del", "rm"},
		Short:   "Delete a hosted file",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			var out map[string]bool
			if err := c.doJSON(http.MethodDelete, siteAPIPath(args[0], "files")+"?path="+url.QueryEscape(args[1]), nil, &out); err != nil {
				return err
			}
			return ctx.writeJSON(cmd, out)
		},
	})
	return cmd
}

func apiUploadsCommand(ctx *commandContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uploads",
		Short: "List, upload, fetch, and delete uploaded files",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list <site>",
		Short: "List uploads",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			var out []uploadInfo
			if err := c.doJSON(http.MethodGet, siteAPIPath(args[0], "uploads"), nil, &out); err != nil {
				return err
			}
			return ctx.writeJSON(cmd, out)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "upload <site> <file>",
		Short: "Upload a file",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			var out uploadResult
			if err := c.doMultipartFile(siteAPIPath(args[0], "uploads"), "file", args[1], &out); err != nil {
				return err
			}
			return ctx.writeJSON(cmd, out)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "fetch <site> <name-or-url>",
		Short: "Fetch an uploaded file",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, config, err := ctx.client()
			if err != nil {
				return err
			}
			name := uploadNameFromCLI(args[1])
			b, _, err := c.doRaw(http.MethodGet, "/uploads/"+url.PathEscape(config.Tenant)+"/"+url.PathEscape(args[0])+"/"+url.PathEscape(name), nil, "", "")
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(b)
			return err
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:     "delete <site> <name-or-url>",
		Aliases: []string{"del", "rm"},
		Short:   "Delete an upload",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			name := uploadNameFromCLI(args[1])
			var out map[string]bool
			if err := c.doJSON(http.MethodDelete, siteAPIPath(args[0], "uploads")+"?name="+url.QueryEscape(name), nil, &out); err != nil {
				return err
			}
			return ctx.writeJSON(cmd, out)
		},
	})
	return cmd
}

func apiAICommand(ctx *commandContext) *cobra.Command {
	var instructions string
	var model string
	var maxOutputTokens int
	cmd := &cobra.Command{
		Use:   "ai <site> <prompt|@file|->",
		Short: "Call the optional site AI endpoint",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			prompt, err := readStringArgument(args[1], cmd.InOrStdin())
			if err != nil {
				return err
			}
			in := aiRequest{Prompt: prompt, Instructions: instructions, Model: model}
			if maxOutputTokens > 0 {
				in.MaxOutputTokens = maxOutputTokens
			}
			var out aiResponse
			if err := c.doJSON(http.MethodPost, siteAPIPath(args[0], "ai"), in, &out); err != nil {
				return err
			}
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, out)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), out.Text)
			return err
		},
	}
	cmd.Flags().StringVar(&instructions, "instructions", "", "system/developer instructions for the model")
	cmd.Flags().StringVar(&model, "model", "", "model override")
	cmd.Flags().IntVar(&maxOutputTokens, "max-output-tokens", 0, "maximum output tokens")
	return cmd
}

func siteAPIPath(site, suffix string) string {
	return "/api/sites/" + url.PathEscape(site) + "/" + strings.TrimLeft(suffix, "/")
}

func readJSONArgument(value string, stdin io.Reader) (any, error) {
	b, err := readArgumentBytes(value, stdin)
	if err != nil {
		return nil, err
	}
	var out any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("invalid JSON value: %w", err)
	}
	return normalizeJSONNumbers(out), nil
}

func readStringArgument(value string, stdin io.Reader) (string, error) {
	b, err := readArgumentBytes(value, stdin)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func readBytesArgument(value string, stdin io.Reader) ([]byte, error) {
	return readArgumentBytes(value, stdin)
}

func readArgumentBytes(value string, stdin io.Reader) ([]byte, error) {
	switch {
	case value == "-":
		return io.ReadAll(stdin)
	case strings.HasPrefix(value, "@"):
		return os.ReadFile(strings.TrimPrefix(value, "@"))
	default:
		return []byte(value), nil
	}
}

func normalizeJSONNumbers(value any) any {
	switch v := value.(type) {
	case json.Number:
		if i, err := strconv.ParseInt(string(v), 10, 64); err == nil {
			return i
		}
		if f, err := strconv.ParseFloat(string(v), 64); err == nil {
			return f
		}
		return string(v)
	case []any:
		for i := range v {
			v[i] = normalizeJSONNumbers(v[i])
		}
		return v
	case map[string]any:
		for key := range v {
			v[key] = normalizeJSONNumbers(v[key])
		}
		return v
	default:
		return value
	}
}

func uploadNameFromCLI(value string) string {
	clean := strings.TrimRight(strings.Split(value, "?")[0], "/")
	if clean == "" {
		return value
	}
	if i := strings.LastIndex(clean, "/"); i >= 0 {
		return clean[i+1:]
	}
	return clean
}

func (c *client) doMultipartFile(path, fieldName, filename string, out any) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile(fieldName, filepath.Base(filename))
	if err != nil {
		return err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	return c.doBytes(http.MethodPost, path, body.Bytes(), mw.FormDataContentType(), out)
}
