package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

const agentRoomName = "__flink_agent"

func agentCommand(ctx *commandContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Enable and listen for site agent messages",
	}
	cmd.AddCommand(agentEnableCommand(ctx, true))
	cmd.AddCommand(agentEnableCommand(ctx, false))
	cmd.AddCommand(agentStatusCommand(ctx))
	cmd.AddCommand(agentListenCommand(ctx))
	cmd.AddCommand(agentRespondCommand(ctx))
	return cmd
}

func agentEnableCommand(ctx *commandContext, enabled bool) *cobra.Command {
	use := "enable <site>"
	short := "Enable the owner-only agent message widget"
	if !enabled {
		use = "disable <site>"
		short = "Disable the agent message widget"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			var status agentStatus
			if err := c.doJSON(http.MethodPut, agentStatusPath(args[0]), map[string]bool{"enabled": enabled}, &status); err != nil {
				return err
			}
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, status)
			}
			printAgentStatus(cmd, args[0], status)
			return nil
		},
	}
}

func agentStatusCommand(ctx *commandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "status <site>",
		Short: "Show agent message status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			var status agentStatus
			if err := c.doJSON(http.MethodGet, agentStatusPath(args[0]), nil, &status); err != nil {
				return err
			}
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, status)
			}
			printAgentStatus(cmd, args[0], status)
			return nil
		},
	}
}

func agentListenCommand(ctx *commandContext) *cobra.Command {
	var once bool
	cmd := &cobra.Command{
		Use:   "listen <site>",
		Short: "Block and listen for site agent widget messages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, config, err := ctx.client()
			if err != nil {
				return err
			}
			return listenForAgentMessages(cmd, ctx, c, config, args[0], once)
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "return after the first message instead of staying attached")
	return cmd
}

func agentRespondCommand(ctx *commandContext) *cobra.Command {
	return &cobra.Command{
		Use:   "respond <site> <message|@file|->",
		Short: "Send a response back to the site agent widget",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := ctx.client()
			if err != nil {
				return err
			}
			text, err := readStringArgument(args[1], cmd.InOrStdin())
			if err != nil {
				return err
			}
			var response agentResponse
			if err := c.doJSON(http.MethodPost, agentResponsesPath(args[0]), map[string]string{"text": text}, &response); err != nil {
				return err
			}
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, response)
			}
			printSections(cmd.OutOrStdout(), "Agent response",
				outputSection{Title: "Target", Rows: []outputRow{
					row("Site", response.Site),
					row("Created", formatTime(response.CreatedAt)),
				}},
				outputSection{Title: "Response", Rows: []outputRow{
					row("Text", response.Text),
				}},
			)
			return nil
		},
	}
}

func listenForAgentMessages(cmd *cobra.Command, ctx *commandContext, c *client, config resolvedConfig, site string, once bool) error {
	seen := map[string]bool{}
	for {
		stop, err := drainStoredAgentMessages(cmd, ctx, c, config, site, once, seen)
		if err != nil || stop {
			return err
		}
		err = streamAgentMessages(config, site, func(msg agentMessage) (bool, error) {
			return handleAgentMessage(cmd, ctx, c, config, site, msg, once, seen)
		})
		if once || err == nil {
			return err
		}
		if _, printErr := fmt.Fprintf(cmd.ErrOrStderr(), "agent listener disconnected: %v; reconnecting in 1s\n", err); printErr != nil {
			return printErr
		}
		time.Sleep(time.Second)
	}
}

func drainStoredAgentMessages(cmd *cobra.Command, ctx *commandContext, c *client, config resolvedConfig, site string, once bool, seen map[string]bool) (bool, error) {
	var messages []agentMessage
	if err := c.doJSON(http.MethodGet, agentMessagesPath(site), nil, &messages); err != nil {
		return false, err
	}
	for _, msg := range messages {
		stop, err := handleAgentMessage(cmd, ctx, c, config, site, msg, once, seen)
		if err != nil || stop {
			return stop, err
		}
	}
	return false, nil
}

func streamAgentMessages(config resolvedConfig, site string, handle func(agentMessage) (bool, error)) error {
	u, err := url.Parse(config.Server)
	if err != nil {
		return err
	}
	u.Scheme = mapHTTPToWS(u.Scheme)
	u.Path = strings.TrimRight(u.Path, "/") + "/ws/" + url.PathEscape(site) + "/" + url.PathEscape(agentRoomName)
	u.RawQuery = ""
	u.Fragment = ""
	header := http.Header{"Authorization": []string{basicAuthHeader(config.Tenant, config.Password)}}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		return err
	}
	defer conn.Close()
	for {
		_, b, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var msg agentMessage
		if err := json.Unmarshal(b, &msg); err != nil {
			continue
		}
		if msg.ID != "" {
			stop, err := handle(msg)
			if err != nil || stop {
				return err
			}
		}
	}
}

func handleAgentMessage(cmd *cobra.Command, ctx *commandContext, c *client, config resolvedConfig, site string, msg agentMessage, once bool, seen map[string]bool) (bool, error) {
	if seen[msg.ID] {
		return false, nil
	}
	seen[msg.ID] = true
	if msg.Site == "" {
		msg.Site = site
	}
	if err := ackAgentMessage(c, site, msg.ID); err != nil {
		return false, err
	}
	if ctx.wantsJSON() {
		if once {
			return true, ctx.writeJSON(cmd, msg)
		}
		return false, json.NewEncoder(cmd.OutOrStdout()).Encode(msg)
	}
	printAgentMessage(cmd, config, msg, once)
	return once, nil
}

func ackAgentMessage(c *client, site, id string) error {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	var out map[string]bool
	return c.doJSON(http.MethodDelete, agentMessagesPath(site)+"/"+url.PathEscape(id), nil, &out)
}

func agentStatusPath(site string) string {
	return "/api/sites/" + url.PathEscape(site) + "/agent"
}

func agentMessagesPath(site string) string {
	return "/api/sites/" + url.PathEscape(site) + "/agent/messages"
}

func agentResponsesPath(site string) string {
	return "/api/sites/" + url.PathEscape(site) + "/agent/responses"
}

func mapHTTPToWS(scheme string) string {
	if scheme == "https" {
		return "wss"
	}
	return "ws"
}

func basicAuthHeader(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func printAgentStatus(cmd *cobra.Command, site string, status agentStatus) {
	printSections(cmd.OutOrStdout(), "Agent messages",
		outputSection{Title: "Target", Rows: []outputRow{
			row("Site", site),
		}},
		outputSection{Title: "Status", Rows: []outputRow{
			row("Enabled", status.Enabled),
			row("Listening", status.Listening),
			row("Pending", status.Pending),
		}},
	)
}

func printAgentMessage(cmd *cobra.Command, config resolvedConfig, msg agentMessage, once bool) {
	screenshotPath, screenshotErr := writeAgentScreenshot(msg)
	rows := []outputRow{
		row("Site", msg.Site),
		row("Sender", firstNonEmpty(msg.Sender, "unknown")),
		row("Received", formatTime(msg.CreatedAt)),
	}
	if screenshotPath != "" {
		rows = append(rows, row("Screenshot", screenshotPath))
	} else if screenshotErr != nil {
		rows = append(rows, row("Screenshot", screenshotErr.Error()))
	}
	nextRows := []outputRow{
		row("Respond", fmt.Sprintf("flink agent respond %s <message>", msg.Site)),
		row("Update", fmt.Sprintf("edit the site, then run flink publish <path> --site %s", msg.Site)),
	}
	if once {
		nextRows = append(nextRows, row("Listen", fmt.Sprintf("flink agent listen %s", msg.Site)))
	} else {
		nextRows = append(nextRows, row("Listening", "this command remains attached; keep it running for the next message"))
	}
	nextRows = append(nextRows, row("URL", canonicalSiteURL(config.Server, config.Tenant, msg.Site)))
	printSections(cmd.OutOrStdout(), "Agent message",
		outputSection{Title: "Target", Rows: rows},
		outputSection{Title: "Message", Rows: []outputRow{
			row("Text", msg.Text),
		}},
		outputSection{Title: "Next", Rows: nextRows},
	)
}

func writeAgentScreenshot(msg agentMessage) (string, error) {
	if msg.Screenshot == nil || strings.TrimSpace(msg.Screenshot.DataURL) == "" {
		return "", nil
	}
	mediaType, data, ok := strings.Cut(msg.Screenshot.DataURL, ",")
	if !ok || !strings.Contains(mediaType, ";base64") {
		return "", fmt.Errorf("unsupported screenshot data")
	}
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("invalid screenshot data")
	}
	dir := filepath.Join(os.TempDir(), "flink-agent-screenshots")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	ext := ".png"
	switch strings.ToLower(msg.Screenshot.Type) {
	case "image/jpeg", "image/jpg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	}
	name := msg.Site + "-" + msg.ID + ext
	path := filepath.Join(dir, name)
	return path, os.WriteFile(path, b, 0644)
}
