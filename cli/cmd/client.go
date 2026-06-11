package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

func newClient(rawURL, username, password string) (*client, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("missing server URL")
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return nil, missingClientAuthError(rawURL, "missing tenant username; pass --tenant or set FLINK_TENANT")
	}
	if strings.TrimSpace(password) == "" {
		return nil, missingClientAuthError(rawURL, "missing tenant password; pass --password or set FLINK_PASSWORD")
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

func (c *client) doBytes(method, path string, body []byte, contentType string, out any) error {
	req, err := http.NewRequest(method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
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

func missingClientAuthError(rawURL, message string) error {
	if hint := clientConfigHint(rawURL); hint != "" {
		return fmt.Errorf("%s\n%s", message, hint)
	}
	return fmt.Errorf("%s", message)
}

func clientConfigHint(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	u.Path = strings.TrimRight(u.Path, "/")
	baseURL := strings.TrimRight(u.String(), "/")
	siteURL := baseURL + "/t/<tenant>/s/<site>/"
	host := u.Hostname()
	if u.Port() == "" && u.EscapedPath() == "" && host != "localhost" && net.ParseIP(host) == nil {
		siteURL = u.Scheme + "://<tenant>--<site>." + host + "/"
	}
	return "Set FLINK_SERVER=" + baseURL + ", FLINK_TENANT, and FLINK_PASSWORD.\nPublished sites use " + siteURL
}
