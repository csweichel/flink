package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

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
