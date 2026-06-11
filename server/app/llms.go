package app

import (
	"bytes"
	"embed"
	"net/http"
	"strings"
	"text/template"
	"time"
)

//go:embed llms.txt.tmpl
var llmsTemplates embed.FS

var llmsTemplate = template.Must(template.ParseFS(llmsTemplates, "llms.txt.tmpl"))

func (a *App) handleLLMSTXT(w http.ResponseWriter, r *http.Request) {
	a.serveLLMSTXT(w, r)
}

func (a *App) serveLLMSTXT(w http.ResponseWriter, r *http.Request) {
	content, err := a.llmsTXT(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeContent(w, r, "llms.txt", time.Time{}, strings.NewReader(content))
}

func (a *App) llmsTXT(r *http.Request) (string, error) {
	origin := requestOrigin(r)
	cliBase := "https://github.com/csweichel/flink/releases/latest/download/"
	data := struct {
		Origin         string
		CLIBase        string
		BaseHost       string
		DomainHosting  bool
		SiteURLPattern string
		IndexURL       string
		AssetURL       string
		DocsURL        string
	}{
		Origin:        origin,
		CLIBase:       cliBase,
		BaseHost:      a.baseHost,
		DomainHosting: a.baseHost != "",
	}
	if a.baseHost != "" {
		data.SiteURLPattern = "https://<tenant>--<site>." + a.baseHost + "/"
		data.IndexURL = "https://<tenant>--<site>." + a.baseHost + "/"
		data.AssetURL = "https://<tenant>--<site>." + a.baseHost + "/assets/app.css"
		data.DocsURL = "https://<tenant>--<site>." + a.baseHost + "/docs/"
	} else {
		data.SiteURLPattern = origin + "/t/<tenant>/s/<site>/"
		data.IndexURL = origin + "/t/<tenant>/s/<site>/"
		data.AssetURL = origin + "/t/<tenant>/s/<site>/assets/app.css"
		data.DocsURL = origin + "/t/<tenant>/s/<site>/docs/"
	}

	var buf bytes.Buffer
	if err := llmsTemplate.ExecuteTemplate(&buf, "llms.txt.tmpl", data); err != nil {
		return "", err
	}
	return buf.String(), nil
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
