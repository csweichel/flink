package app

import (
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

	"flink/server/api"
	"flink/server/frontend"
	"flink/server/storage"
)

type Config struct {
	DataDir       string `yaml:"dataDir"`
	StorageDriver string `yaml:"storage"`
	BaseHost      string `yaml:"baseHost"`
}

type App struct {
	config   Config
	backend  storage.Backend
	store    *api.Store
	hub      *api.Hub
	baseHost string
	mux      *http.ServeMux
}

func New(config Config) *App {
	app := &App{
		config:   config,
		hub:      api.NewHub(),
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
	a.mux.HandleFunc("/api/public/", a.requireTenant(a.handlePublicAPI))
	a.mux.HandleFunc("/flink.js", func(w http.ResponseWriter, r *http.Request) {
		b, err := frontend.ReadClientJS()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		http.ServeContent(w, r, "flink.js", time.Time{}, bytes.NewReader(b))
	})
	a.mux.HandleFunc("/uploads/", a.requireTenant(a.handleUploadFile))
	a.mux.HandleFunc("/ws/", a.requireTenant(a.handleWS))
	a.mux.HandleFunc("/", a.requireTenant(a.handleSite))
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
		_, _ = io.WriteString(w, loginHTML(""))
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, loginHTML(err.Error()))
			return
		}
		username := r.Form.Get("username")
		password := r.Form.Get("password")
		tenant, err := a.store.AuthenticateTenant(username, password)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, loginHTML(err.Error()))
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
		tenant, err := a.store.RegisterTenant(r.Form.Get("username"), r.Form.Get("password"))
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

func loginHTML(message string) string {
	return authHTML("Sign in", "/_flink/login", "Sign in", message, true)
}

func registerHTML(message string) string {
	return authHTML("Register tenant", "/_flink/register", "Request access", message, false)
}

func pendingHTML(username string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Flink pending</title>` + authCSS() + `</head><body><main><h1>Flink</h1><p>Registration for <strong>` + html.EscapeString(username) + `</strong> is pending approval.</p><p><a href="/_flink/login">Back to sign in</a></p></main></body></html>`
}

func authHTML(title, action, button, message string, showRegister bool) string {
	extra := `<p><a href="/_flink/register">Request a tenant account</a></p>`
	if !showRegister {
		extra = `<p><a href="/_flink/login">Back to sign in</a></p>`
	}
	if message != "" {
		message = `<p class="error">` + html.EscapeString(message) + `</p>`
	}
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Flink ` + title + `</title>` + authCSS() + `</head><body><main><form method="post" action="` + action + `"><h1>Flink</h1><h2>` + title + `</h2>` + message + `<input name="username" autocomplete="username" placeholder="tenant" pattern="[a-z0-9-]+" autofocus required><input name="password" type="password" autocomplete="current-password" placeholder="password" required><button>` + button + `</button>` + extra + `</form></main></body></html>`
}

func authCSS() string {
	return `<style>body{font-family:ui-sans-serif,system-ui,sans-serif;margin:0;display:grid;min-height:100vh;place-items:center;background:#f7f7f4;color:#171717}main,form{width:min(380px,calc(100vw - 32px));display:grid;gap:12px}h1,h2,p{margin:0}h1{font-size:28px}h2{font-size:16px;font-weight:600;color:#525252}input,button{font:inherit;padding:12px 14px;border:1px solid #c9c9c2;border-radius:6px}button{background:#151515;color:white;cursor:pointer}.error{color:#b91c1c}a{color:#0f766e}</style>`
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
	slug, area, tail, ok := parseAPIPath(strings.TrimPrefix(r.URL.Path, "/api/public/"))
	if !ok {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid public API path"))
		return
	}
	a.dispatchAPI(w, r, slug, area, tail)
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

func (a *App) dispatchAPI(w http.ResponseWriter, r *http.Request, slug, area, tail string) {
	tenant := tenantFromContext(r.Context())
	switch area {
	case "files":
		a.handleFiles(w, r, tenant.Username, slug)
	case "data":
		a.handleData(w, r, tenant.Username, slug, tail)
	case "uploads":
		a.handleUpload(w, r, tenant.Username, slug)
	case "ai":
		a.handleAI(w, r)
	default:
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown API area"))
	}
}

func (a *App) handleFiles(w http.ResponseWriter, r *http.Request, tenant, slug string) {
	p, err := api.CleanPath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		b, err := a.store.ReadSiteFile(tenant, slug, p)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeJSON(w, map[string]string{"path": p, "content": string(b)}, nil)
	case http.MethodPut, http.MethodPost:
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
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
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
}

func (a *App) handleAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	client := api.NewAIClientFromEnv()
	if !client.Configured() {
		writeJSON(w, api.AIResponse{
			Text:       "AI is not configured. Set OPENAI_API_KEY on the Flink server to enable this endpoint.",
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
	tenant := tenantFromContext(r.Context())
	rest := strings.TrimPrefix(r.URL.Path, "/uploads/")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) != 3 || parts[0] != tenant.Username || !api.ValidSlug(parts[1]) {
		http.NotFound(w, r)
		return
	}
	p, err := api.CleanPath(parts[2])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := a.store.ReadUpload(tenant.Username, parts[1], p)
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
	tenant := tenantFromContext(r.Context())
	rest := strings.TrimPrefix(r.URL.Path, "/ws/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || !api.ValidSlug(parts[0]) {
		http.NotFound(w, r)
		return
	}
	a.hub.ServeRoom(w, r, tenant.Username+"/"+parts[0]+"/"+parts[1])
}

func (a *App) handleSite(w http.ResponseWriter, r *http.Request) {
	authTenant := tenantFromContext(r.Context())
	tenant, slug, sitePath := a.resolveSite(r, authTenant.Username)
	if slug == "" {
		http.Redirect(w, r, "/_flink", http.StatusSeeOther)
		return
	}
	if tenant != authTenant.Username {
		http.NotFound(w, r)
		return
	}
	if sitePath == "" || strings.HasSuffix(sitePath, "/") {
		sitePath = sitePath + "index.html"
	}
	p, err := api.CleanPath(sitePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := a.store.ReadSiteFile(tenant, slug, p)
	if err != nil && p != "index.html" {
		p = "index.html"
		b, err = a.store.ReadSiteFile(tenant, slug, p)
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
