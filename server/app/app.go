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
