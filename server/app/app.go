package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	return nil
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
	// Dashboard and discovery.
	a.mux.HandleFunc("GET /_flink/login", a.handleLogin)
	a.mux.HandleFunc("POST /_flink/login", a.handleLogin)
	a.mux.HandleFunc("GET /_flink/register", a.handleRegister)
	a.mux.HandleFunc("POST /_flink/register", a.handleRegister)
	a.mux.HandleFunc("GET /_flink/logout", a.handleLogout)
	a.mux.HandleFunc("GET /_flink", a.requireTenant(a.dashboard))
	a.mux.HandleFunc("GET /_flink/", a.requireTenant(a.dashboard))
	a.mux.HandleFunc("GET /llms.txt", a.handleLLMSTXT)
	a.mux.HandleFunc("GET /_flink/agent-instructions", a.handleLLMSTXT)
	a.mux.HandleFunc("GET /.well-known/flink.json", a.handleDiscoveryJSON)
	a.mux.HandleFunc("GET /flink-logo.png", a.handleLogo)
	a.mux.HandleFunc("GET /favicon.ico", a.handleLogo)
	a.mux.HandleFunc("GET /flink.js", func(w http.ResponseWriter, r *http.Request) {
		b, err := frontend.ReadClientJS()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		http.ServeContent(w, r, "flink.js", time.Time{}, bytes.NewReader(b))
	})

	// Tenant-authenticated management API.
	a.mux.HandleFunc("GET /api/auth/me", a.requireTenant(a.handleMe))
	a.mux.HandleFunc("GET /api/sites", a.requireTenant(a.handleListSites))
	a.mux.HandleFunc("POST /api/sites", a.requireTenant(a.handleCreateSite))
	a.mux.HandleFunc("GET /api/sites/{slug}", a.requireTenant(a.handleSiteDetails))
	a.mux.HandleFunc("DELETE /api/sites/{slug}", a.requireTenant(a.handleDeleteSite))
	a.mux.HandleFunc("GET /api/sites/{slug}/auth", a.requireTenant(a.handleGetSiteAuth))
	a.mux.HandleFunc("PUT /api/sites/{slug}/auth", a.requireTenant(a.handleSetSiteAuth))
	a.mux.HandleFunc("POST /api/sites/{slug}/auth", a.requireTenant(a.handleSetSiteAuth))
	a.mux.HandleFunc("GET /api/sites/{slug}/publishes", a.requireTenant(a.handleListPublishes))
	a.mux.HandleFunc("POST /api/sites/{slug}/publishes", a.requireTenant(a.handleRecordPublish))
	a.mux.HandleFunc("POST /api/sites/{slug}/rollback", a.requireTenant(a.handleRollbackPublish))
	a.mux.HandleFunc("GET /api/sites/{slug}/files", a.requireTenant(a.handleTenantFiles))
	a.mux.HandleFunc("PUT /api/sites/{slug}/files", a.requireTenant(a.handleTenantFiles))
	a.mux.HandleFunc("POST /api/sites/{slug}/files", a.requireTenant(a.handleTenantFiles))
	a.mux.HandleFunc("DELETE /api/sites/{slug}/files", a.requireTenant(a.handleTenantFiles))
	a.mux.HandleFunc("GET /api/sites/{slug}/data/{$}", a.requireTenant(a.handleTenantData))
	a.mux.HandleFunc("GET /api/sites/{slug}/data/{key}", a.requireTenant(a.handleTenantData))
	a.mux.HandleFunc("PUT /api/sites/{slug}/data/{key}", a.requireTenant(a.handleTenantData))
	a.mux.HandleFunc("POST /api/sites/{slug}/data/{key}", a.requireTenant(a.handleTenantData))
	a.mux.HandleFunc("DELETE /api/sites/{slug}/data/{key}", a.requireTenant(a.handleTenantData))
	a.mux.HandleFunc("GET /api/sites/{slug}/uploads", a.requireTenant(a.handleTenantUploads))
	a.mux.HandleFunc("POST /api/sites/{slug}/uploads", a.requireTenant(a.handleTenantUploads))
	a.mux.HandleFunc("DELETE /api/sites/{slug}/uploads", a.requireTenant(a.handleTenantUploads))
	a.mux.HandleFunc("GET /api/sites/{slug}/archive", a.requireTenant(a.handleTenantArchive))
	a.mux.HandleFunc("POST /api/sites/{slug}/ai", a.requireTenant(a.handleAI))

	a.mux.Handle("/api/public/", http.StripPrefix("/api/public", a.publicAPIMux()))

	a.mux.HandleFunc("GET /uploads/{tenant}/{slug}/{name...}", a.handleUploadFile)
	a.mux.HandleFunc("/ws/", a.handleWS)
	a.mux.HandleFunc("/", a.handleSite)
}

func (a *App) publicAPIMux() http.Handler {
	mux := http.NewServeMux()

	// Tenantless routes infer the tenant from the authenticated viewer.
	mux.HandleFunc("GET /{slug}/files", a.handlePublicFiles)
	mux.HandleFunc("PUT /{slug}/files", a.handlePublicFiles)
	mux.HandleFunc("POST /{slug}/files", a.handlePublicFiles)
	mux.HandleFunc("DELETE /{slug}/files", a.handlePublicFiles)
	mux.HandleFunc("GET /{slug}/data/{$}", a.handlePublicData)
	mux.HandleFunc("GET /{slug}/data/{key}", a.handlePublicData)
	mux.HandleFunc("PUT /{slug}/data/{key}", a.handlePublicData)
	mux.HandleFunc("POST /{slug}/data/{key}", a.handlePublicData)
	mux.HandleFunc("DELETE /{slug}/data/{key}", a.handlePublicData)
	mux.HandleFunc("GET /{slug}/uploads", a.handlePublicUploads)
	mux.HandleFunc("POST /{slug}/uploads", a.handlePublicUploads)
	mux.HandleFunc("DELETE /{slug}/uploads", a.handlePublicUploads)
	mux.HandleFunc("GET /{slug}/archive", a.handlePublicArchive)
	mux.HandleFunc("POST /{slug}/ai", a.handlePublicAI)

	// Canonical tenant-qualified browser API routes.
	mux.HandleFunc("GET /t/{tenant}/s/{slug}/files", a.handlePublicTenantFiles)
	mux.HandleFunc("PUT /t/{tenant}/s/{slug}/files", a.handlePublicTenantFiles)
	mux.HandleFunc("POST /t/{tenant}/s/{slug}/files", a.handlePublicTenantFiles)
	mux.HandleFunc("DELETE /t/{tenant}/s/{slug}/files", a.handlePublicTenantFiles)
	mux.HandleFunc("GET /t/{tenant}/s/{slug}/data/{$}", a.handlePublicTenantData)
	mux.HandleFunc("GET /t/{tenant}/s/{slug}/data/{key}", a.handlePublicTenantData)
	mux.HandleFunc("PUT /t/{tenant}/s/{slug}/data/{key}", a.handlePublicTenantData)
	mux.HandleFunc("POST /t/{tenant}/s/{slug}/data/{key}", a.handlePublicTenantData)
	mux.HandleFunc("DELETE /t/{tenant}/s/{slug}/data/{key}", a.handlePublicTenantData)
	mux.HandleFunc("GET /t/{tenant}/s/{slug}/uploads", a.handlePublicTenantUploads)
	mux.HandleFunc("POST /t/{tenant}/s/{slug}/uploads", a.handlePublicTenantUploads)
	mux.HandleFunc("DELETE /t/{tenant}/s/{slug}/uploads", a.handlePublicTenantUploads)
	mux.HandleFunc("GET /t/{tenant}/s/{slug}/archive", a.handlePublicTenantArchive)
	mux.HandleFunc("POST /t/{tenant}/s/{slug}/ai", a.handlePublicTenantAI)

	return mux
}

func (a *App) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	tenant, slug, name := r.PathValue("tenant"), r.PathValue("slug"), r.PathValue("name")
	if !api.ValidSlug(tenant) || !api.ValidSlug(slug) || name == "" {
		http.NotFound(w, r)
		return
	}
	if !a.authorizeSiteAPI(w, r, tenant, slug) {
		return
	}
	p, err := api.CleanPath(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := a.store.ReadUpload(tenant, slug, p)
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
		if r.URL.Path == "/" && (r.Method == http.MethodHead || (r.Method == http.MethodGet && !wantsHTML(r))) {
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
