package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/csweichel/flink/server/api"
)

func (a *App) handleListSites(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	sites, err := a.store.ListSites(tenant.Username)
	writeJSON(w, sites, err)
}

func (a *App) handleCreateSite(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
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
}

func (a *App) handleSiteDetails(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	slug, ok := validPathSlug(w, r)
	if !ok {
		return
	}
	details, err := a.store.ReadSiteDetails(tenant.Username, slug)
	writeJSON(w, details, err)
}

func (a *App) handleDeleteSite(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	slug, ok := validPathSlug(w, r)
	if !ok {
		return
	}
	writeJSON(w, map[string]bool{"deleted": true}, a.store.DeleteSite(tenant.Username, slug))
}

func (a *App) handleGetSiteAuth(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	slug, ok := validPathSlug(w, r)
	if !ok {
		return
	}
	meta, err := a.store.ReadMeta(tenant.Username, slug)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, meta.Auth, nil)
}

func (a *App) handleSetSiteAuth(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	slug, ok := validPathSlug(w, r)
	if !ok {
		return
	}
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
}

func (a *App) handleListPublishes(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	slug, ok := validPathSlug(w, r)
	if !ok {
		return
	}
	records, err := a.store.ListPublishes(tenant.Username, slug)
	writeJSON(w, records, err)
}

func (a *App) handleRecordPublish(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	slug, ok := validPathSlug(w, r)
	if !ok {
		return
	}
	var record api.PublishRecord
	if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := a.store.RecordPublish(tenant.Username, slug, record)
	writeJSON(w, out, err)
}

func (a *App) handleRollbackPublish(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	slug, ok := validPathSlug(w, r)
	if !ok {
		return
	}
	var in struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	record, err := a.store.RollbackPublish(tenant.Username, slug, in.Version)
	writeJSON(w, record, err)
}

func (a *App) handleTenantFiles(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	slug, ok := validPathSlug(w, r)
	if !ok {
		return
	}
	a.handleFiles(w, r, tenant.Username, slug)
}

func (a *App) handleTenantData(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	slug, ok := validPathSlug(w, r)
	if !ok {
		return
	}
	a.handleData(w, r, tenant.Username, slug, r.PathValue("key"))
}

func (a *App) handleTenantUploads(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	slug, ok := validPathSlug(w, r)
	if !ok {
		return
	}
	a.handleUpload(w, r, tenant.Username, slug)
}

func (a *App) handleTenantArchive(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	slug, ok := validPathSlug(w, r)
	if !ok {
		return
	}
	a.handleArchive(w, r, tenant.Username, slug)
}

func (a *App) handlePublicFiles(w http.ResponseWriter, r *http.Request) {
	tenant, slug, ok := a.ownerPublicSite(w, r)
	if !ok {
		return
	}
	a.handleFiles(w, r, tenant, slug)
}

func (a *App) handlePublicData(w http.ResponseWriter, r *http.Request) {
	tenant, slug, ok := a.authorizedPublicSite(w, r)
	if !ok {
		return
	}
	a.handleData(w, r, tenant, slug, r.PathValue("key"))
}

func (a *App) handlePublicUploads(w http.ResponseWriter, r *http.Request) {
	tenant, slug, ok := a.authorizedPublicSite(w, r)
	if !ok {
		return
	}
	a.handleUpload(w, r, tenant, slug)
}

func (a *App) handlePublicArchive(w http.ResponseWriter, r *http.Request) {
	tenant, slug, ok := a.ownerPublicSite(w, r)
	if !ok {
		return
	}
	a.handleArchive(w, r, tenant, slug)
}

func (a *App) handlePublicAI(w http.ResponseWriter, r *http.Request) {
	_, _, ok := a.authorizedPublicSite(w, r)
	if !ok {
		return
	}
	a.handleAI(w, r)
}

func (a *App) handlePublicTenantFiles(w http.ResponseWriter, r *http.Request) {
	a.handlePublicFiles(w, r)
}

func (a *App) handlePublicTenantData(w http.ResponseWriter, r *http.Request) {
	a.handlePublicData(w, r)
}

func (a *App) handlePublicTenantUploads(w http.ResponseWriter, r *http.Request) {
	a.handlePublicUploads(w, r)
}

func (a *App) handlePublicTenantArchive(w http.ResponseWriter, r *http.Request) {
	a.handlePublicArchive(w, r)
}

func (a *App) handlePublicTenantAI(w http.ResponseWriter, r *http.Request) {
	a.handlePublicAI(w, r)
}

func validPathSlug(w http.ResponseWriter, r *http.Request) (string, bool) {
	slug := r.PathValue("slug")
	if !api.ValidSlug(slug) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid site slug"))
		return "", false
	}
	return slug, true
}

func (a *App) ownerPublicSite(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	authTenant, authenticated := a.authenticate(r)
	if !authenticated {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
		return "", "", false
	}
	tenant := r.PathValue("tenant")
	if tenant == "" {
		tenant = authTenant.Username
	} else if tenant != authTenant.Username {
		http.NotFound(w, r)
		return "", "", false
	}
	slug := r.PathValue("slug")
	if !api.ValidSlug(tenant) || !api.ValidSlug(slug) {
		http.NotFound(w, r)
		return "", "", false
	}
	return tenant, slug, true
}

func (a *App) authorizedPublicSite(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	tenant := r.PathValue("tenant")
	slug := r.PathValue("slug")
	if tenant == "" {
		authTenant, authenticated := a.authenticate(r)
		if !authenticated {
			writeError(w, http.StatusUnauthorized, fmt.Errorf("authentication required"))
			return "", "", false
		}
		tenant = authTenant.Username
	}
	if !a.authorizeSiteAPI(w, r, tenant, slug) {
		return "", "", false
	}
	return tenant, slug, true
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
