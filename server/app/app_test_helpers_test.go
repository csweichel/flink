package app

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testApp(t *testing.T) *App {
	t.Helper()
	return testAppWithConfig(t, Config{DataDir: t.TempDir(), BaseHost: "quick.internal"})
}

func testAppWithConfig(t *testing.T, config Config) *App {
	t.Helper()
	a := New(config)
	if err := a.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.CreateApprovedTenant(testTenant, testPassword); err != nil {
		t.Fatal(err)
	}
	return a
}

func postJSON(t *testing.T, h http.Handler, url string, v any) {
	t.Helper()
	putJSONMethod(t, h, http.MethodPost, url, v)
}

func putJSON(t *testing.T, h http.Handler, url string, v any) {
	t.Helper()
	putJSONMethod(t, h, http.MethodPut, url, v)
}

func putJSONMethod(t *testing.T, h http.Handler, method, url string, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	res := request(t, h, method, url, bytes.NewReader(b), "application/json")
	if res.Code < 200 || res.Code > 299 {
		t.Fatalf("%s %s failed: %d %s", method, url, res.Code, res.Body.String())
	}
}

func rawPutJSON(t *testing.T, h http.Handler, url string, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	res := rawRequest(t, h, http.MethodPut, url, bytes.NewReader(b), "application/json")
	if res.Code < 200 || res.Code > 299 {
		t.Fatalf("PUT %s failed: %d %s", url, res.Code, res.Body.String())
	}
}

func request(t *testing.T, h http.Handler, method, url string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	return requestAs(t, h, testTenant, testPassword, method, url, body, contentType)
}

func requestAs(t *testing.T, h http.Handler, username, password, method, url string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.SetBasicAuth(username, password)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func rawRequest(t *testing.T, h http.Handler, method, url string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func basicAuth(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func readZip(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[file.Name] = content
	}
	return out
}
