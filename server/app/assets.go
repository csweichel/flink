package app

import (
	"bytes"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/csweichel/flink/server/frontend"
)

func (a *App) handleLogo(w http.ResponseWriter, r *http.Request) {
	b, err := frontend.ReadLogoPNG()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	http.ServeContent(w, r, "flink-logo.png", time.Time{}, bytes.NewReader(b))
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
