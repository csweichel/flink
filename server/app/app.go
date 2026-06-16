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
	DropTenantDomainPrefix    bool   `yaml:"dropTenantDomainPrefix"`
	DropTenantDomainPrefixSet bool   `yaml:"-"`
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
	mcp      http.Handler
}

func New(config Config) *App {
	if !config.DropTenantDomainPrefixSet {
		config.DropTenantDomainPrefix = true
	}
	app := &App{
		config:   config,
		hub:      api.NewHub(),
		aiClient: api.NewAIClient(config.AI),
		baseHost: strings.TrimPrefix(strings.ToLower(config.BaseHost), "."),
		mux:      http.NewServeMux(),
	}
	app.mcp = app.newMCPHandler()
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
	a.mux.HandleFunc("GET /_flink/codex-plugin.sh", a.handleCodexPluginScript)
	a.mux.HandleFunc("GET /_flink", a.requireTenant(a.dashboard))
	a.mux.HandleFunc("GET /_flink/", a.requireTenant(a.dashboard))
	a.mux.HandleFunc("GET /llms.txt", a.handleLLMSTXT)
	a.mux.HandleFunc("GET /_flink/agent-instructions", a.handleLLMSTXT)
	a.mux.HandleFunc("GET /.well-known/flink.json", a.handleDiscoveryJSON)
	a.mux.HandleFunc("GET /flink-logo.png", a.handleLogo)
	a.mux.HandleFunc("GET /favicon.ico", a.handleLogo)
	a.mux.HandleFunc("POST /mcp", a.requireTenant(a.handleMCP))
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
	a.mux.HandleFunc("GET /api/sites/{slug}/agent", a.requireTenant(a.handleGetSiteAgent))
	a.mux.HandleFunc("PUT /api/sites/{slug}/agent", a.requireTenant(a.handleSetSiteAgent))
	a.mux.HandleFunc("POST /api/sites/{slug}/agent", a.requireTenant(a.handleSetSiteAgent))
	a.mux.HandleFunc("GET /api/sites/{slug}/agent/messages", a.requireTenant(a.handleListAgentMessages))
	a.mux.HandleFunc("DELETE /api/sites/{slug}/agent/messages/{id}", a.requireTenant(a.handleDeleteAgentMessage))
	a.mux.HandleFunc("GET /api/sites/{slug}/agent/responses", a.requireTenant(a.handleListAgentResponses))
	a.mux.HandleFunc("POST /api/sites/{slug}/agent/responses", a.requireTenant(a.handleCreateAgentResponse))
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
	mux.HandleFunc("GET /{slug}/agent", a.handlePublicAgentStatus)
	mux.HandleFunc("POST /{slug}/agent/messages", a.handlePublicAgentMessage)
	mux.HandleFunc("GET /{slug}/agent/responses", a.handlePublicAgentResponses)

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
	mux.HandleFunc("GET /t/{tenant}/s/{slug}/agent", a.handlePublicTenantAgentStatus)
	mux.HandleFunc("POST /t/{tenant}/s/{slug}/agent/messages", a.handlePublicTenantAgentMessage)
	mux.HandleFunc("GET /t/{tenant}/s/{slug}/agent/responses", a.handlePublicTenantAgentResponses)

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
	ct := mime.TypeByExtension(filepath.Ext(p))
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if shouldInjectAgentWidget(ct, p) {
		if meta, err := a.store.ReadMeta(tenant, slug); err == nil && meta.AgentMessages && meta.Auth.Mode == api.SiteAuthOwner {
			b = injectAgentWidget(b, tenant, slug)
			http.ServeContent(w, r, filepath.Base(p), time.Now(), bytes.NewReader(b))
			return
		}
	}
	http.ServeContent(w, r, filepath.Base(p), time.Now(), bytes.NewReader(b))
}

func shouldInjectAgentWidget(contentType, p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".html" || ext == ".htm" || strings.HasPrefix(strings.ToLower(contentType), "text/html")
}

func injectAgentWidget(page []byte, tenant, slug string) []byte {
	if bytes.Contains(page, []byte("data-flink-agent-widget")) {
		return page
	}
	widget := agentWidgetHTML(tenant, slug)
	lower := bytes.ToLower(page)
	if i := bytes.LastIndex(lower, []byte("</body>")); i >= 0 {
		out := make([]byte, 0, len(page)+len(widget))
		out = append(out, page[:i]...)
		out = append(out, widget...)
		out = append(out, page[i:]...)
		return out
	}
	return append(append([]byte{}, page...), widget...)
}

func agentWidgetHTML(tenant, slug string) []byte {
	tenantJSON, _ := json.Marshal(tenant)
	slugJSON, _ := json.Marshal(slug)
	return []byte(`<style data-flink-agent-widget>
#flink-agent-widget{position:fixed;right:16px;bottom:16px;z-index:2147483647;width:min(340px,calc(100vw - 32px));font:14px/1.4 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#17202a;background:#fff;border:1px solid #cfd7df;box-shadow:0 12px 32px rgba(15,23,42,.18);border-radius:8px;padding:12px}
#flink-agent-widget[data-collapsed=true]{width:58px;height:58px;padding:0;border-radius:999px;overflow:hidden}
#flink-agent-widget-toggle{width:100%;border:0;border-radius:999px;background:#17202a;color:#fff;font:700 12px/1 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;padding:10px;cursor:pointer}
#flink-agent-widget[data-collapsed=false] #flink-agent-widget-toggle{width:auto;border-radius:6px;margin-bottom:8px;background:#eef2f5;color:#17202a}
#flink-agent-widget[data-collapsed=true] #flink-agent-widget-panel{display:none}
#flink-agent-widget form{display:grid;gap:8px;margin:0}
#flink-agent-widget textarea{box-sizing:border-box;width:100%;min-height:76px;resize:vertical;border:1px solid #aeb8c2;border-radius:6px;padding:8px;font:inherit;color:inherit;background:#fff}
#flink-agent-widget .flink-agent-send{border:0;border-radius:6px;background:#1957d2;color:#fff;font:600 14px/1.2 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;padding:9px 10px;cursor:pointer}
#flink-agent-widget .flink-agent-send:disabled{background:#8da4d8;cursor:not-allowed}
#flink-agent-widget .flink-agent-secondary{border:1px solid #aeb8c2;border-radius:6px;background:#fff;color:#17202a;font:600 13px/1.2 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;padding:8px 10px;cursor:pointer}
#flink-agent-widget-status{display:flex;align-items:center;gap:8px;margin-bottom:8px;font-size:12px;color:#46515c}
#flink-agent-widget-status i{width:9px;height:9px;border-radius:999px;background:#aeb8c2;display:inline-block}
#flink-agent-widget[data-listening=true] #flink-agent-widget-status i{background:#1b8f4d}
#flink-agent-widget-responses{display:grid;gap:6px;margin:0 0 8px}
#flink-agent-widget-responses article{border-left:3px solid #1957d2;background:#f5f8fb;border-radius:4px;padding:7px 8px;font-size:12px;color:#26313d}
#flink-agent-widget-result{min-height:18px;font-size:12px;color:#46515c}
</style><div id="flink-agent-widget" data-flink-agent-widget data-listening="false" data-collapsed="false"><button id="flink-agent-widget-toggle" type="button" aria-expanded="true">Agent</button><div id="flink-agent-widget-panel"><div id="flink-agent-widget-status"><i></i><span>Checking agent...</span></div><div id="flink-agent-widget-responses"></div><form><textarea name="message" placeholder="Message the agent" required></textarea><button class="flink-agent-secondary" type="button" id="flink-agent-screenshot">Include screenshot</button><button class="flink-agent-send">Send to agent</button><div id="flink-agent-widget-result" role="status" aria-live="polite"></div></form></div></div><script data-flink-agent-widget>
(() => {
  const tenant = ` + string(tenantJSON) + `;
  const site = ` + string(slugJSON) + `;
  const root = document.getElementById("flink-agent-widget");
  if (!root) return;
  const toggle = root.querySelector("#flink-agent-widget-toggle");
  const responses = root.querySelector("#flink-agent-widget-responses");
  const statusText = root.querySelector("#flink-agent-widget-status span");
  const form = root.querySelector("form");
  const textarea = root.querySelector("textarea");
  const button = root.querySelector(".flink-agent-send");
  const screenshotButton = root.querySelector("#flink-agent-screenshot");
  const result = root.querySelector("#flink-agent-widget-result");
  const apiBase = "/api/public/t/" + encodeURIComponent(tenant) + "/s/" + encodeURIComponent(site) + "/agent";
  const listeningKey = "flink-agent-listening:" + tenant + ":" + site;
  const collapsedKey = "flink-agent-collapsed:" + tenant + ":" + site;
  const responseKey = "flink-agent-response:" + tenant + ":" + site;
  let screenshot = null;
  function setCollapsed(collapsed) {
    root.dataset.collapsed = collapsed ? "true" : "false";
    toggle.setAttribute("aria-expanded", collapsed ? "false" : "true");
    sessionStorage.setItem(collapsedKey, collapsed ? "true" : "false");
  }
  setCollapsed(sessionStorage.getItem(collapsedKey) === "true");
  toggle.addEventListener("click", () => setCollapsed(root.dataset.collapsed !== "true"));
  function renderResponses(items) {
    responses.replaceChildren();
    const latest = items.at(-1);
    if (!latest) return;
    for (const item of [latest]) {
      const article = document.createElement("article");
      article.textContent = item.text;
      responses.append(article);
    }
  }
  async function loadResponses() {
    try {
      const res = await fetch(apiBase + "/responses", { credentials: "same-origin" });
      if (!res.ok) return;
      const items = await res.json();
      const latest = items.at(-1);
      const previous = sessionStorage.getItem(responseKey);
      if (latest?.id) {
        sessionStorage.setItem(responseKey, latest.id);
        if (previous && previous !== latest.id) {
          location.reload();
          return;
        }
      }
      renderResponses(items);
    } catch {}
  }
  async function captureScreenshot() {
    if (!navigator.mediaDevices?.getDisplayMedia) {
      throw new Error("Screenshot capture is not available in this browser.");
    }
    const stream = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: false });
    try {
      const video = document.createElement("video");
      video.srcObject = stream;
      await video.play();
      const canvas = document.createElement("canvas");
      canvas.width = video.videoWidth || innerWidth;
      canvas.height = video.videoHeight || innerHeight;
      canvas.getContext("2d").drawImage(video, 0, 0, canvas.width, canvas.height);
      return { name: "screenshot.png", type: "image/png", dataUrl: canvas.toDataURL("image/png") };
    } finally {
      stream.getTracks().forEach((track) => track.stop());
    }
  }
  async function refresh() {
    try {
      const res = await fetch(apiBase, { credentials: "same-origin" });
      if (!res.ok) throw new Error("status failed");
      const status = await res.json();
      const wasListening = sessionStorage.getItem(listeningKey);
      sessionStorage.setItem(listeningKey, status.listening ? "true" : "false");
      if (wasListening === "false" && status.listening) {
        location.reload();
        return;
      }
      root.dataset.listening = status.listening ? "true" : "false";
      statusText.textContent = status.listening ? "Agent listening" : "Agent offline";
    } catch {
      root.dataset.listening = "false";
      statusText.textContent = "Agent status unknown";
    }
  }
  screenshotButton.addEventListener("click", async () => {
    screenshotButton.disabled = true;
    result.textContent = "Choose this browser tab or window to attach a screenshot.";
    try {
      screenshot = await captureScreenshot();
      result.textContent = "Screenshot attached.";
    } catch (err) {
      result.textContent = err instanceof Error ? err.message : "Screenshot failed.";
    } finally {
      screenshotButton.disabled = false;
    }
  });
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const text = textarea.value.trim();
    if (!text) return;
    button.disabled = true;
    result.textContent = "Sending...";
    try {
      const res = await fetch(apiBase + "/messages", {
        method: "POST",
        credentials: "same-origin",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(screenshot ? { text, screenshot } : { text }),
      });
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || "message failed");
      textarea.value = "";
      screenshot = null;
      result.textContent = root.dataset.listening === "true" ? "Sent to agent." : "Saved for the next agent.";
      refresh();
    } catch (err) {
      result.textContent = err instanceof Error ? err.message : "Message failed.";
    } finally {
      button.disabled = false;
    }
  });
  refresh();
  loadResponses();
  setInterval(refresh, 3000);
  setInterval(loadResponses, 5000);
})();
</script>`)
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
			if api.ValidSlug(label) && a.config.DropTenantDomainPrefix {
				if defaultTenant == "" {
					if tenant, ok := a.resolveTenantlessDomainSite(label); ok {
						return tenant, label, strings.TrimPrefix(r.URL.Path, "/")
					}
					return "", "", ""
				}
				return defaultTenant, label, strings.TrimPrefix(r.URL.Path, "/")
			}
		}
	}
	return "", "", ""
}

func (a *App) resolveTenantlessDomainSite(slug string) (string, bool) {
	tenants, err := a.store.ListTenants(api.TenantApproved)
	if err != nil {
		return "", false
	}
	found := ""
	for _, tenant := range tenants {
		sites, err := a.store.ListSites(tenant.Username)
		if err != nil {
			continue
		}
		for _, site := range sites {
			if site.Slug == slug {
				if found != "" {
					return "", false
				}
				found = tenant.Username
				break
			}
		}
	}
	return found, found != ""
}

func (a *App) siteURL(origin, tenant, slug string) string {
	if a.baseHost == "" {
		return strings.TrimRight(origin, "/") + "/t/" + tenant + "/s/" + slug + "/"
	}
	if a.config.DropTenantDomainPrefix {
		return "https://" + slug + "." + a.baseHost + "/"
	}
	return "https://" + tenant + "--" + slug + "." + a.baseHost + "/"
}

func (a *App) siteURLTemplate(origin string) string {
	if a.baseHost == "" {
		return strings.TrimRight(origin, "/") + "/t/{tenant}/s/{site}/"
	}
	if a.config.DropTenantDomainPrefix {
		return "https://{site}." + a.baseHost + "/"
	}
	return "https://{tenant}--{site}." + a.baseHost + "/"
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
