package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/csweichel/flink/server/api"
	"github.com/csweichel/flink/server/frontend"
	"github.com/csweichel/flink/server/storage"
)

type Config struct {
	DataDir                   string `yaml:"dataDir"`
	StorageDriver             string `yaml:"storage"`
	BaseHost                  string `yaml:"baseHost"`
	AutoApproveTenants        bool   `yaml:"autoApproveTenants"`
	DisableTenantRegistration bool   `yaml:"disableTenantRegistration"`
	DefaultSiteAuthMode       string `yaml:"defaultSiteAuthMode"`
	AI                        api.AIConfig
}

type App struct {
	config   Config
	backend  storage.Backend
	store    *api.Store
	hub      *api.Hub
	aiClient *api.AIClient
	baseHost string
	mux      *http.ServeMux
}

func New(config Config) *App {
	app := &App{
		config:   config,
		hub:      api.NewHub(),
		aiClient: api.NewAIClient(config.AI),
		baseHost: strings.TrimPrefix(strings.ToLower(config.BaseHost), "."),
		mux:      http.NewServeMux(),
	}
	app.routes()
	return app
}

func (a *App) Init() error {
	backend, err := storage.Open(a.config.StorageDriver, a.config.DataDir)
	if err != nil {
		return err
	}
	if err := backend.Init(context.Background()); err != nil {
		return err
	}
	a.backend = backend
	a.store = api.NewStore(backend, frontend.DefaultIndex())
	if err := a.store.SetDefaultSiteAuthMode(a.config.DefaultSiteAuthMode); err != nil {
		_ = backend.Close()
		a.backend = nil
		a.store = nil
		return err
	}
	return a.store.Init()
}

func (a *App) Close() error {
	if a.backend == nil {
		return nil
	}
	return a.backend.Close()
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.ServeHTTP(w, r)
}

func (a *App) routes() {
	a.mux.HandleFunc("/_flink/login", a.handleLogin)
	a.mux.HandleFunc("/_flink/register", a.handleRegister)
	a.mux.HandleFunc("/_flink/logout", a.handleLogout)
	a.mux.HandleFunc("/_flink", a.requireTenant(a.dashboard))
	a.mux.HandleFunc("/_flink/", a.requireTenant(a.dashboard))
	a.mux.HandleFunc("/api/auth/me", a.requireTenant(a.handleMe))
	a.mux.HandleFunc("/api/sites", a.requireTenant(a.handleSites))
	a.mux.HandleFunc("/api/sites/", a.requireTenant(a.handleSiteAPI))
	a.mux.HandleFunc("/api/public/", a.handlePublicAPI)
	a.mux.HandleFunc("/llms.txt", a.handleLLMSTXT)
	a.mux.HandleFunc("/flink-logo.png", a.handleLogo)
	a.mux.HandleFunc("/favicon.ico", a.handleLogo)
	a.mux.HandleFunc("/flink.js", func(w http.ResponseWriter, r *http.Request) {
		b, err := frontend.ReadClientJS()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		http.ServeContent(w, r, "flink.js", time.Time{}, bytes.NewReader(b))
	})
	a.mux.HandleFunc("/uploads/", a.handleUploadFile)
	a.mux.HandleFunc("/ws/", a.handleWS)
	a.mux.HandleFunc("/", a.handleSite)
}

func (a *App) handleLLMSTXT(w http.ResponseWriter, r *http.Request) {
	a.serveLLMSTXT(w, r)
}

func (a *App) serveLLMSTXT(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	http.ServeContent(w, r, "llms.txt", time.Time{}, strings.NewReader(a.llmsTXT(r)))
}

func (a *App) llmsTXT(r *http.Request) string {
	origin := requestOrigin(r)
	cliBase := "https://github.com/csweichel/flink/releases/latest/download/"
	if a.baseHost != "" {
		baseHost := a.baseHost
		return fmt.Sprintf(`# Flink agent instructions

You are looking at a Flink server. Flink is a simple application server built for agents to publish prototype apps and single-page apps as live static websites with built-in browser APIs for JSON storage, uploads, realtime rooms, and optional AI calls.

If a user asks you to publish or deploy a prototype app to this server, do the work yourself with the Flink CLI. Do not ask the user to clone this repository, build Flink, choose infrastructure, or set up a separate backend.

Inputs you need:

- FLINK_SERVER: %s
- FLINK_TENANT: the tenant username
- FLINK_PASSWORD: the tenant password
- site name: a lowercase slug such as my-site
- files to publish: usually ./dist for a built app, or ./index.html for a single-file prototype

Use this exact URL pattern for published websites:
https://<tenant>--<site>.%s/

For example, tenant demo and site my-site will be available at:

https://demo--my-site.%s/

Download the Flink CLI from GitHub Releases. Do not ask the user to clone the repository or build the CLI from source.

- Linux amd64: %sflink_linux_amd64.tar.gz
- Linux arm64: %sflink_linux_arm64.tar.gz
- macOS amd64: %sflink_darwin_amd64.tar.gz
- macOS arm64: %sflink_darwin_arm64.tar.gz

Install and configure the CLI:

curl -L -o flink.tar.gz %sflink_linux_amd64.tar.gz
tar -xzf flink.tar.gz

export FLINK_SERVER=%s
export FLINK_TENANT=<tenant>
export FLINK_PASSWORD=<password>

Deployment flow:

1. Build the app into static files if it has a build step.
2. Create the site if it does not already exist.
3. Publish either the build directory or the single HTML file.
4. Open the live URL and verify the app works.

Publish a built app or static directory:

./flink site create my-site
./flink site write my-site ./dist

Publish a single-page HTML prototype:

./flink site create my-site
./flink site write my-site ./index.html index.html

Site access uses this server's default site auth mode when created. To make a site visible without signing in, run:

./flink site auth my-site none

To require any approved tenant login, run:

./flink site auth my-site tenants

To restrict to specific tenants, run:

./flink site auth my-site tenants <tenant>...

For multi-file websites, publish a directory. Paths are preserved under the same domain:

- ./dist/index.html -> https://<tenant>--<site>.%s/
- ./dist/assets/app.css -> https://<tenant>--<site>.%s/assets/app.css
- ./dist/docs/index.html -> https://<tenant>--<site>.%s/docs/

Hosted sites can use Flink backend APIs by importing:

<script src="/flink.js"></script>

Use Flink storage, uploads, realtime rooms, and AI from browser code instead of creating a custom backend. Keep the first deployed version usable, then iterate by publishing updated files with the same site write command.
`, origin, baseHost, baseHost, cliBase, cliBase, cliBase, cliBase, cliBase, origin, baseHost, baseHost, baseHost)
	}
	return fmt.Sprintf(`# Flink agent instructions

You are looking at a Flink server. Flink is a simple application server built for agents to publish prototype apps and single-page apps as live static websites with built-in browser APIs for JSON storage, uploads, realtime rooms, and optional AI calls.

If a user asks you to publish or deploy a prototype app to this server, do the work yourself with the Flink CLI. Do not ask the user to clone this repository, build Flink, choose infrastructure, or set up a separate backend.

This Flink server does not have domain-based hosting configured, so use path-based tenant URLs.

Inputs you need:

- FLINK_SERVER: %s
- FLINK_TENANT: the tenant username
- FLINK_PASSWORD: the tenant password
- site name: a lowercase slug such as my-site
- files to publish: usually ./dist for a built app, or ./index.html for a single-file prototype

Use this exact URL pattern for published websites:
%s/t/<tenant>/s/<site>/

Download the Flink CLI from GitHub Releases. Do not ask the user to clone the repository or build the CLI from source.

- Linux amd64: %sflink_linux_amd64.tar.gz
- Linux arm64: %sflink_linux_arm64.tar.gz
- macOS amd64: %sflink_darwin_amd64.tar.gz
- macOS arm64: %sflink_darwin_arm64.tar.gz

Install and configure the CLI:

curl -L -o flink.tar.gz %sflink_linux_amd64.tar.gz
tar -xzf flink.tar.gz

export FLINK_SERVER=%s
export FLINK_TENANT=<tenant>
export FLINK_PASSWORD=<password>

Deployment flow:

1. Build the app into static files if it has a build step.
2. Create the site if it does not already exist.
3. Publish either the build directory or the single HTML file.
4. Open the live URL and verify the app works.

Publish a built app or static directory:

./flink site create my-site
./flink site write my-site ./dist

Publish a single-page HTML prototype:

./flink site create my-site
./flink site write my-site ./index.html index.html

Site access uses this server's default site auth mode when created. To make a site visible without signing in, run:

./flink site auth my-site none

To require any approved tenant login, run:

./flink site auth my-site tenants

To restrict to specific tenants, run:

./flink site auth my-site tenants <tenant>...

For multi-file websites, publish a directory. Paths are preserved under the same site base:

- ./dist/index.html -> %s/t/<tenant>/s/<site>/
- ./dist/assets/app.css -> %s/t/<tenant>/s/<site>/assets/app.css
- ./dist/docs/index.html -> %s/t/<tenant>/s/<site>/docs/

Hosted sites can use Flink backend APIs by importing:

<script src="/flink.js"></script>

Use Flink storage, uploads, realtime rooms, and AI from browser code instead of creating a custom backend. Keep the first deployed version usable, then iterate by publishing updated files with the same site write command.
`, origin, origin, cliBase, cliBase, cliBase, cliBase, cliBase, origin, origin, origin, origin)
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

func (a *App) handleLogo(w http.ResponseWriter, r *http.Request) {
	b, err := frontend.ReadLogoPNG()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	http.ServeContent(w, r, "flink-logo.png", time.Time{}, bytes.NewReader(b))
}

func (a *App) requireTenant(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := a.authenticate(r)
		if ok {
			next(w, r.WithContext(context.WithValue(r.Context(), tenantContextKey{}, tenant)))
			return
		}
		if wantsHTML(r) || r.URL.Path == "/_flink" || r.URL.Path == "/_flink/" {
			http.Redirect(w, r, "/_flink/login", http.StatusSeeOther)
			return
		}
		writeError(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
	}
}

type tenantContextKey struct{}

func tenantFromContext(ctx context.Context) api.PublicTenant {
	if tenant, ok := ctx.Value(tenantContextKey{}).(api.PublicTenant); ok {
		return tenant
	}
	return api.PublicTenant{}
}

func (a *App) authenticate(r *http.Request) (api.PublicTenant, bool) {
	if tenant := tenantFromContext(r.Context()); tenant.Username != "" {
		return tenant, true
	}
	if username, password, ok := r.BasicAuth(); ok {
		tenant, err := a.store.AuthenticateTenant(username, password)
		return tenant, err == nil
	}
	if c, err := r.Cookie("flink_session"); err == nil {
		session, err := a.store.ReadSession(c.Value)
		if err != nil {
			return api.PublicTenant{}, false
		}
		meta, err := a.store.ReadTenant(session.Username)
		if err != nil || meta.Status != api.TenantApproved {
			return api.PublicTenant{}, false
		}
		return meta.Public(), true
	}
	return api.PublicTenant{}, false
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, loginHTML("", !a.config.DisableTenantRegistration))
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, loginHTML(err.Error(), !a.config.DisableTenantRegistration))
			return
		}
		username := r.Form.Get("username")
		password := r.Form.Get("password")
		tenant, err := a.store.AuthenticateTenant(username, password)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, loginHTML(err.Error(), !a.config.DisableTenantRegistration))
			return
		}
		session, err := a.store.CreateSession(tenant.Username, 7*24*time.Hour)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "flink_session", Value: session.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, "/_flink", http.StatusSeeOther)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	if a.config.DisableTenantRegistration {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, registerHTML(""))
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, registerHTML(err.Error()))
			return
		}
		username := r.Form.Get("username")
		password := r.Form.Get("password")
		if a.config.AutoApproveTenants {
			tenant, err := a.store.RegisterApprovedTenant(username, password)
			if err != nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, registerHTML(err.Error()))
				return
			}
			session, err := a.store.CreateSession(tenant.Username, 7*24*time.Hour)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "flink_session", Value: session.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
			http.Redirect(w, r, "/_flink", http.StatusSeeOther)
			return
		}
		tenant, err := a.store.RegisterTenant(username, password)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, registerHTML(err.Error()))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, pendingHTML(tenant.Username))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("flink_session"); err == nil {
		_ = a.store.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "flink_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/_flink/login", http.StatusSeeOther)
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, tenantFromContext(r.Context()), nil)
}

func wantsHTML(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html")
}

func loginHTML(message string, showRegister bool) string {
	return authHTML("Sign in", "/_flink/login", "Sign in", message, showRegister)
}

func registerHTML(message string) string {
	return authHTML("Register tenant", "/_flink/register", "Request access", message, false)
}

func pendingHTML(username string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Flink pending</title><link rel="icon" type="image/png" href="/flink-logo.png">` + authCSS() + `</head><body><main><img class="logo" src="/flink-logo.png" alt="Flink"><h1>Flink</h1><p>Registration for <strong>` + html.EscapeString(username) + `</strong> is pending approval.</p><p><a href="/_flink/login">Back to sign in</a></p></main></body></html>`
}

func authHTML(title, action, button, message string, showRegister bool) string {
	extra := ""
	if action == "/_flink/login" && showRegister {
		extra = `<p><a href="/_flink/register">Request a tenant account</a></p>`
	}
	if action == "/_flink/register" {
		extra = `<p><a href="/_flink/login">Back to sign in</a></p>`
	}
	if message != "" {
		message = `<p class="error">` + html.EscapeString(message) + `</p>`
	}
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Flink ` + title + `</title><link rel="icon" type="image/png" href="/flink-logo.png">` + authCSS() + `</head><body><main><form method="post" action="` + action + `"><img class="logo" src="/flink-logo.png" alt="Flink"><h1>Flink</h1><h2>` + title + `</h2>` + message + `<input name="username" autocomplete="username" placeholder="tenant" pattern="[a-z0-9-]+" autofocus required><input name="password" type="password" autocomplete="current-password" placeholder="password" required><button>` + button + `</button>` + extra + `</form></main><footer>Made with ❤️ by <a href="https://csweichel.de" rel="noreferrer">csweichel.de</a> - find it on <a href="https://github.com/csweichel/flink" rel="noreferrer">GitHub</a></footer></body></html>`
}

func authCSS() string {
	return `<style>body{font-family:ui-sans-serif,system-ui,sans-serif;margin:0;display:grid;min-height:100vh;place-items:center;background:#f7f7f4;color:#171717}main,form{width:min(380px,calc(100vw - 32px));display:grid;gap:12px}.logo{width:112px;height:112px;object-fit:contain;display:block;margin:0 0 4px}h1,h2,p{margin:0}h1{font-size:28px}h2{font-size:16px;font-weight:600;color:#525252}input,button{font:inherit;padding:12px 14px;border:1px solid #c9c9c2;border-radius:6px}button{background:#151515;color:white;cursor:pointer}.error{color:#b91c1c}footer{position:fixed;left:16px;right:16px;bottom:18px;text-align:center;font-size:13px;color:#737373}a{color:#0f766e}</style>`
}

func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/_flink/")
	if r.URL.Path == "/_flink" || name == "" {
		name = "index.html"
	}
	b, servedName, err := frontend.ReadDist(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ct := mime.TypeByExtension(filepath.Ext(servedName)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, filepath.Base(servedName), time.Time{}, bytes.NewReader(b))
}

func (a *App) handleSites(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	switch r.Method {
	case http.MethodGet:
		sites, err := a.store.ListSites(tenant.Username)
		writeJSON(w, sites, err)
	case http.MethodPost:
		var in struct {
			Slug  string `json:"slug"`
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		meta, err := a.store.CreateSite(tenant.Username, in.Slug, in.Title)
		writeJSON(w, meta, err)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) handleSiteAPI(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sites/"), "/")
	if slug, ok := strings.CutSuffix(rest, "/auth"); ok && api.ValidSlug(slug) {
		tenant := tenantFromContext(r.Context())
		switch r.Method {
		case http.MethodGet:
			meta, err := a.store.ReadMeta(tenant.Username, slug)
			if err != nil {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeJSON(w, meta.Auth, nil)
		case http.MethodPut, http.MethodPost:
			var policy api.SiteAuthPolicy
			if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			meta, err := a.store.UpdateSiteAuth(tenant.Username, slug, policy)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, meta.Auth, nil)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}
	if r.Method == http.MethodDelete && api.ValidSlug(rest) {
		tenant := tenantFromContext(r.Context())
		writeJSON(w, map[string]bool{"deleted": true}, a.store.DeleteSite(tenant.Username, rest))
		return
	}
	slug, area, tail, ok := parseAPIPath(rest)
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid site API path"))
		return
	}
	a.dispatchAPI(w, r, slug, area, tail)
}

func (a *App) handlePublicAPI(w http.ResponseWriter, r *http.Request) {
	tenant, slug, area, tail, ok := parsePublicAPIPath(strings.TrimPrefix(r.URL.Path, "/api/public/"))
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid public API path"))
		return
	}
	authTenant, authenticated := a.authenticate(r)
	if tenant == "" {
		if !authenticated {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
			return
		}
		tenant = authTenant.Username
	}
	if area == "files" || area == "archive" {
		if !authenticated {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
			return
		}
		if authTenant.Username != tenant {
			http.NotFound(w, r)
			return
		}
		a.dispatchAPIForTenant(w, r, tenant, slug, area, tail)
		return
	}
	if !a.authorizeSiteAPI(w, r, tenant, slug) {
		return
	}
	a.dispatchAPIForTenant(w, r, tenant, slug, area, tail)
}

func parseAPIPath(rest string) (slug, area, tail string, ok bool) {
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || !api.ValidSlug(parts[0]) {
		return "", "", "", false
	}
	if len(parts) == 3 {
		tail = parts[2]
	}
	return parts[0], parts[1], tail, true
}

func parsePublicAPIPath(rest string) (tenant, slug, area, tail string, ok bool) {
	rest = strings.Trim(rest, "/")
	tenantParts := strings.SplitN(rest, "/", 6)
	if len(tenantParts) >= 5 && tenantParts[0] == "t" && tenantParts[2] == "s" && api.ValidSlug(tenantParts[1]) && api.ValidSlug(tenantParts[3]) && isAPIArea(tenantParts[4]) {
		if len(tenantParts) == 6 {
			tail = tenantParts[5]
		}
		return tenantParts[1], tenantParts[3], tenantParts[4], tail, true
	}
	parts := strings.SplitN(rest, "/", 4)
	if len(parts) < 2 || !api.ValidSlug(parts[0]) {
		return "", "", "", "", false
	}
	if isAPIArea(parts[1]) {
		if len(parts) == 3 {
			tail = parts[2]
		}
		return "", parts[0], parts[1], tail, true
	}
	if len(parts) < 3 || !api.ValidSlug(parts[1]) || !isAPIArea(parts[2]) {
		return "", "", "", "", false
	}
	if len(parts) == 4 {
		tail = parts[3]
	}
	return parts[0], parts[1], parts[2], tail, true
}

func isAPIArea(area string) bool {
	switch area {
	case "files", "data", "uploads", "archive", "ai":
		return true
	default:
		return false
	}
}

func (a *App) dispatchAPI(w http.ResponseWriter, r *http.Request, slug, area, tail string) {
	tenant := tenantFromContext(r.Context())
	a.dispatchAPIForTenant(w, r, tenant.Username, slug, area, tail)
}

func (a *App) dispatchAPIForTenant(w http.ResponseWriter, r *http.Request, tenant, slug, area, tail string) {
	switch area {
	case "files":
		a.handleFiles(w, r, tenant, slug)
	case "data":
		a.handleData(w, r, tenant, slug, tail)
	case "uploads":
		a.handleUpload(w, r, tenant, slug)
	case "archive":
		a.handleArchive(w, r, tenant, slug)
	case "ai":
		a.handleAI(w, r)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown API area"))
	}
}

func (a *App) handleFiles(w http.ResponseWriter, r *http.Request, tenant, slug string) {
	switch r.Method {
	case http.MethodGet:
		if !r.URL.Query().Has("path") {
			prefix, err := api.CleanPrefix(r.URL.Query().Get("prefix"))
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			files, err := a.store.ListSiteFiles(tenant, slug, prefix)
			writeJSON(w, files, err)
			return
		}
		p, err := api.CleanPath(r.URL.Query().Get("path"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		b, err := a.store.ReadSiteFile(tenant, slug, p)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, map[string]string{"path": p, "content": string(b)}, nil)
	case http.MethodPut, http.MethodPost:
		p, err := api.CleanPath(r.URL.Query().Get("path"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if r.Header.Get("Content-Type") == "application/json" {
			var in struct {
				Content string `json:"content"`
			}
			if err := json.Unmarshal(b, &in); err == nil {
				b = []byte(in.Content)
			}
		}
		writeJSON(w, map[string]string{"path": p}, a.store.WriteSiteFile(tenant, slug, p, b))
	case http.MethodDelete:
		p, err := api.CleanPath(r.URL.Query().Get("path"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, map[string]bool{"deleted": true}, a.store.DeleteSiteFile(tenant, slug, p))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) handleData(w http.ResponseWriter, r *http.Request, tenant, slug, key string) {
	key = strings.Trim(key, "/")
	if key == "" && r.Method != http.MethodGet {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing key"))
		return
	}
	store, err := a.store.ReadData(tenant, slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if key == "" {
			writeJSON(w, store, nil)
			return
		}
		v, ok := store[key]
		if !ok {
			writeError(w, http.StatusNotFound, fmt.Errorf("key not found"))
			return
		}
		writeJSON(w, v, nil)
	case http.MethodPut, http.MethodPost:
		var v any
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		store[key] = v
		writeJSON(w, v, a.store.WriteData(tenant, slug, store))
	case http.MethodDelete:
		delete(store, key)
		writeJSON(w, map[string]bool{"deleted": true}, a.store.WriteData(tenant, slug, store))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) handleUpload(w http.ResponseWriter, r *http.Request, tenant, slug string) {
	switch r.Method {
	case http.MethodGet:
		uploads, err := a.store.ListUploads(tenant, slug)
		writeJSON(w, uploads, err)
	case http.MethodPost:
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		f, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		defer f.Close()
		uploaded, err := a.store.SaveUpload(tenant, slug, header.Filename, f)
		writeJSON(w, uploaded, err)
	case http.MethodDelete:
		name, err := api.CleanPath(r.URL.Query().Get("name"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, map[string]bool{"deleted": true}, a.store.DeleteUpload(tenant, slug, name))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) handleArchive(w http.ResponseWriter, r *http.Request, tenant, slug string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	b, err := a.siteArchive(tenant, slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, slug))
	http.ServeContent(w, r, slug+".zip", time.Now(), bytes.NewReader(b))
}

func (a *App) siteArchive(tenant, slug string) ([]byte, error) {
	meta, err := a.store.ReadMeta(tenant, slug)
	if err != nil {
		return nil, err
	}
	files, err := a.store.ListSiteFiles(tenant, slug, "")
	if err != nil {
		return nil, err
	}
	data, err := a.store.ReadData(tenant, slug)
	if err != nil {
		return nil, err
	}
	uploads, err := a.store.ListUploads(tenant, slug)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := writeZipJSON(zw, "site.json", meta); err != nil {
		return nil, err
	}
	if err := writeZipJSON(zw, "data.json", data); err != nil {
		return nil, err
	}
	for _, file := range files {
		b, err := a.store.ReadSiteFile(tenant, slug, file.Path)
		if err != nil {
			return nil, err
		}
		if err := writeZipFile(zw, "files/"+file.Path, b); err != nil {
			return nil, err
		}
	}
	for _, upload := range uploads {
		b, err := a.store.ReadUpload(tenant, slug, upload.Name)
		if err != nil {
			return nil, err
		}
		if err := writeZipFile(zw, "uploads/"+upload.Name, b); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeZipJSON(zw *zip.Writer, name string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return writeZipFile(zw, name, b)
}

func writeZipFile(zw *zip.Writer, name string, b []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func (a *App) handleAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	client := a.aiClient
	if !client.Configured() {
		writeJSON(w, api.AIResponse{
			Text:       "AI is not configured. Set ai.apiKey in the Flink server config to enable this endpoint.",
			Configured: false,
		}, nil)
		return
	}
	var in api.AIRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := client.Generate(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, out, nil)
}

func (a *App) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/uploads/")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) != 3 || !api.ValidSlug(parts[0]) || !api.ValidSlug(parts[1]) {
		http.NotFound(w, r)
		return
	}
	if !a.authorizeSiteAPI(w, r, parts[0], parts[1]) {
		return
	}
	p, err := api.CleanPath(parts[2])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := a.store.ReadUpload(parts[0], parts[1], p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ct := mime.TypeByExtension(filepath.Ext(p)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, filepath.Base(p), time.Time{}, bytes.NewReader(b))
}

func (a *App) handleWS(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/ws/")
	parts := strings.SplitN(rest, "/", 3)
	var tenant, slug, room string
	switch {
	case len(parts) == 2 && api.ValidSlug(parts[0]):
		authTenant, ok := a.authenticate(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
			return
		}
		tenant, slug, room = authTenant.Username, parts[0], parts[1]
	case len(parts) == 3 && api.ValidSlug(parts[0]) && api.ValidSlug(parts[1]):
		tenant, slug, room = parts[0], parts[1], parts[2]
	default:
		http.NotFound(w, r)
		return
	}
	if !a.authorizeSiteAPI(w, r, tenant, slug) {
		return
	}
	a.hub.ServeRoom(w, r, tenant+"/"+slug+"/"+room)
}

func (a *App) handleSite(w http.ResponseWriter, r *http.Request) {
	authTenant, authenticated := a.authenticate(r)
	defaultTenant := ""
	if authenticated {
		defaultTenant = authTenant.Username
	}
	tenant, slug, sitePath := a.resolveSite(r, defaultTenant)
	if slug == "" {
		if r.Method == http.MethodGet && r.URL.Path == "/" && !wantsHTML(r) {
			a.serveLLMSTXT(w, r)
			return
		}
		http.Redirect(w, r, "/_flink", http.StatusSeeOther)
		return
	}
	if !a.authorizeSitePage(w, r, tenant, slug) {
		return
	}
	originalSitePath := sitePath
	if sitePath == "" || strings.HasSuffix(sitePath, "/") {
		sitePath = sitePath + "index.html"
	}
	p, err := api.CleanPath(sitePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := a.store.ReadSiteFile(tenant, slug, p)
	if err != nil {
		if !strings.HasSuffix(sitePath, "/") && filepath.Ext(p) == "" {
			dirIndex := strings.TrimSuffix(p, "/") + "/index.html"
			b, err = a.store.ReadSiteFile(tenant, slug, dirIndex)
			if err == nil {
				p = dirIndex
			}
		}
		if err != nil && p != "index.html" && filepath.Ext(originalSitePath) == "" {
			p = "index.html"
			b, err = a.store.ReadSiteFile(tenant, slug, p)
		}
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ct := mime.TypeByExtension(filepath.Ext(p)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, filepath.Base(p), time.Now(), bytes.NewReader(b))
}

func (a *App) authorizeSitePage(w http.ResponseWriter, r *http.Request, tenant, slug string) bool {
	if !api.ValidSlug(tenant) || !api.ValidSlug(slug) {
		http.NotFound(w, r)
		return false
	}
	meta, err := a.store.ReadMeta(tenant, slug)
	if err != nil {
		http.NotFound(w, r)
		return false
	}
	authTenant, authenticated := a.authenticate(r)
	if meta.Auth.Allows(tenant, authTenant.Username, authenticated) {
		return true
	}
	if !authenticated {
		http.Redirect(w, r, "/_flink/login", http.StatusSeeOther)
		return false
	}
	http.NotFound(w, r)
	return false
}

func (a *App) authorizeSiteAPI(w http.ResponseWriter, r *http.Request, tenant, slug string) bool {
	if !api.ValidSlug(tenant) || !api.ValidSlug(slug) {
		http.NotFound(w, r)
		return false
	}
	meta, err := a.store.ReadMeta(tenant, slug)
	if err != nil {
		http.NotFound(w, r)
		return false
	}
	authTenant, authenticated := a.authenticate(r)
	if meta.Auth.Allows(tenant, authTenant.Username, authenticated) {
		return true
	}
	if !authenticated {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
		return false
	}
	http.NotFound(w, r)
	return false
}

func (a *App) resolveSite(r *http.Request, defaultTenant string) (string, string, string) {
	if strings.HasPrefix(r.URL.Path, "/t/") {
		rest := strings.TrimPrefix(r.URL.Path, "/t/")
		parts := strings.SplitN(rest, "/", 4)
		if len(parts) >= 3 && api.ValidSlug(parts[0]) && parts[1] == "s" && api.ValidSlug(parts[2]) {
			p := ""
			if len(parts) == 4 {
				p = parts[3]
			}
			return parts[0], parts[2], p
		}
	}
	if strings.HasPrefix(r.URL.Path, "/s/") {
		rest := strings.TrimPrefix(r.URL.Path, "/s/")
		parts := strings.SplitN(rest, "/", 2)
		if api.ValidSlug(parts[0]) {
			p := ""
			if len(parts) == 2 {
				p = parts[1]
			}
			return defaultTenant, parts[0], p
		}
	}
	if a.baseHost != "" {
		host := strings.ToLower(r.Host)
		host = strings.Split(host, ":")[0]
		suffix := "." + a.baseHost
		if strings.HasSuffix(host, suffix) {
			label := strings.TrimSuffix(host, suffix)
			parts := strings.SplitN(label, "--", 2)
			if len(parts) == 2 && api.ValidSlug(parts[0]) && api.ValidSlug(parts[1]) {
				return parts[0], parts[1], strings.TrimPrefix(r.URL.Path, "/")
			}
			if api.ValidSlug(label) {
				return defaultTenant, label, strings.TrimPrefix(r.URL.Path, "/")
			}
		}
	}
	return "", "", ""
}

func writeJSON(w http.ResponseWriter, v any, err error) {
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	if errors.Is(err, api.ErrNotFound) {
		code = http.StatusNotFound
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func logListenAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "http://localhost" + addr
	}
	return "http://" + addr
}

func logServing(addr string) {
	log.Printf("flink listening on %s", logListenAddr(addr))
}
