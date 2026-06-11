package app

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/csweichel/flink/server/api"
)

func (a *App) requireTenant(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant, ok := a.authenticate(r)
		if ok {
			next(w, r.WithContext(context.WithValue(r.Context(), tenantContextKey{}, tenant)))
			return
		}
		setDiscoveryHeaders(w, r)
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
		writeMethodNotAllowed(w, r)
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
		writeMethodNotAllowed(w, r)
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
	writeJSON(w, struct {
		api.PublicTenant
		BaseHost string `json:"baseHost,omitempty"`
	}{
		PublicTenant: tenantFromContext(r.Context()),
		BaseHost:     a.baseHost,
	}, nil)
}

func wantsHTML(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html")
}

func writeMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	setDiscoveryHeaders(w, r)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusMethodNotAllowed)
	_, _ = io.WriteString(w, "This is a Flink server. For agent deployment instructions, GET / or fetch /.well-known/flink.json.\n")
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
