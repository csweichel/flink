package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/csweichel/flink/server/api"
)

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
