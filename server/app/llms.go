package app

import (
	"bytes"
	"embed"
	"encoding/json"
	"net/http"
	"strings"
	"text/template"
	"time"
)

//go:embed llms.txt.tmpl
var llmsTemplates embed.FS

var llmsTemplate = template.Must(template.ParseFS(llmsTemplates, "llms.txt.tmpl"))

type flinkDiscovery struct {
	Type              string   `json:"type"`
	Server            string   `json:"server"`
	AgentInstructions string   `json:"agent_instructions"`
	RequiredEnv       []string `json:"required_env"`
	CLI               string   `json:"cli"`
	SiteURLTemplate   string   `json:"site_url_template"`
	Commands          []string `json:"commands"`

	CLIBase        string `json:"-"`
	BaseHost       string `json:"-"`
	DomainHosting  bool   `json:"-"`
	SiteURLPattern string `json:"-"`
	IndexURL       string `json:"-"`
	AssetURL       string `json:"-"`
	DocsURL        string `json:"-"`
}

func (a *App) handleLLMSTXT(w http.ResponseWriter, r *http.Request) {
	a.serveLLMSTXT(w, r)
}

func (a *App) serveLLMSTXT(w http.ResponseWriter, r *http.Request) {
	content, err := a.llmsTXT(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	setDiscoveryHeaders(w, r)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeContent(w, r, "llms.txt", time.Time{}, strings.NewReader(content))
}

func (a *App) handleDiscoveryJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowed(w, r)
		return
	}
	setDiscoveryHeaders(w, r)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(a.discovery(r))
}

func (a *App) llmsTXT(r *http.Request) (string, error) {
	data := a.discovery(r)
	if data.DomainHosting {
		data.SiteURLPattern = "https://<tenant>--<site>." + data.BaseHost + "/"
		data.IndexURL = "https://<tenant>--<site>." + data.BaseHost + "/"
		data.AssetURL = "https://<tenant>--<site>." + data.BaseHost + "/assets/app.css"
		data.DocsURL = "https://<tenant>--<site>." + data.BaseHost + "/docs/"
	} else {
		data.SiteURLPattern = data.Server + "/t/<tenant>/s/<site>/"
		data.IndexURL = data.Server + "/t/<tenant>/s/<site>/"
		data.AssetURL = data.Server + "/t/<tenant>/s/<site>/assets/app.css"
		data.DocsURL = data.Server + "/t/<tenant>/s/<site>/docs/"
	}

	var buf bytes.Buffer
	if err := llmsTemplate.ExecuteTemplate(&buf, "llms.txt.tmpl", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (a *App) discovery(r *http.Request) flinkDiscovery {
	origin := requestOrigin(r)
	cliBase := "https://github.com/csweichel/flink/releases/latest/download/"
	siteURLTemplate := origin + "/t/{tenant}/s/{site}/"
	if a.baseHost != "" {
		siteURLTemplate = "https://{tenant}--{site}." + a.baseHost + "/"
	}
	return flinkDiscovery{
		Type:              "flink",
		Server:            origin,
		AgentInstructions: origin + "/_flink/agent-instructions",
		RequiredEnv:       []string{"FLINK_TENANT", "FLINK_PASSWORD"},
		CLI:               cliBase + "flink_linux_amd64.tar.gz",
		SiteURLTemplate:   siteURLTemplate,
		Commands: []string{
			"flink publish ./dist --site <site>",
			"flink auth <site> none",
			"flink inspect <site>",
		},
		CLIBase:       cliBase,
		BaseHost:      a.baseHost,
		DomainHosting: a.baseHost != "",
	}
}

func setDiscoveryHeaders(w http.ResponseWriter, r *http.Request) {
	origin := requestOrigin(r)
	w.Header().Set("X-Flink-Server", origin)
	w.Header().Set("X-Flink-Agent-Instructions", origin+"/_flink/agent-instructions")
	w.Header().Set("Link", `</.well-known/flink.json>; rel="service-desc"`)
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if forwardedHost := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]); forwardedHost != "" {
		host = forwardedHost
	}
	if host == "" {
		host = "localhost"
	}
	return scheme + "://" + host
}
