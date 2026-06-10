package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/csweichel/flink/shared/banner"
)

func TestSiteCreateUsesTenantBasicAuthAndPrintsTenantURL(t *testing.T) {
	var gotAuthUser, gotAuthPassword string
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/sites" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		gotAuthUser, gotAuthPassword, _ = r.BasicAuth()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(siteMeta{Slug: gotBody["slug"], UpdatedAt: time.Now().UTC()})
	}))
	defer server.Close()

	out, err := runCommand("site", "create", "demo", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if gotAuthUser != "alice" || gotAuthPassword != "secret" {
		t.Fatalf("unexpected auth: %q %q", gotAuthUser, gotAuthPassword)
	}
	if gotBody["slug"] != "demo" {
		t.Fatalf("unexpected body: %#v", gotBody)
	}
	if !strings.Contains(out, server.URL+"/t/alice/s/demo/") {
		t.Fatalf("output should include tenant-scoped URL, got %q", out)
	}
}

func TestSiteWritePublishesFileContentToRequestedPath(t *testing.T) {
	dir := t.TempDir()
	localFile := filepath.Join(dir, "index.html")
	if err := os.WriteFile(localFile, []byte("<h1>hello</h1>"), 0644); err != nil {
		t.Fatal(err)
	}

	var gotContent string
	var gotTargetPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/sites/demo/files" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if user, password, ok := r.BasicAuth(); !ok || user != "alice" || password != "secret" {
			t.Fatalf("missing or wrong auth")
		}
		gotTargetPath = r.URL.Query().Get("path")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		gotContent = string(body)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": gotTargetPath})
	}))
	defer server.Close()

	out, err := runCommand("site", "write", "demo", localFile, "pages/home.html", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if gotTargetPath != "pages/home.html" {
		t.Fatalf("unexpected target path: %q", gotTargetPath)
	}
	if gotContent != "<h1>hello</h1>" {
		t.Fatalf("unexpected content: %q", gotContent)
	}
	if !strings.Contains(out, "published pages/home.html") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestSiteWritePublishesDirectoryTree(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>home</h1>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.css"), []byte("body{color:red}"), 0644); err != nil {
		t.Fatal(err)
	}

	gotFiles := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/sites/demo/files" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if user, password, ok := r.BasicAuth(); !ok || user != "alice" || password != "secret" {
			t.Fatalf("missing or wrong auth")
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		gotFiles[r.URL.Query().Get("path")] = string(b)
		_ = json.NewEncoder(w).Encode(map[string]string{"path": r.URL.Query().Get("path")})
	}))
	defer server.Close()

	out, err := runCommand("site", "write", "demo", dir, "public", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if gotFiles["public/index.html"] != "<h1>home</h1>" || gotFiles["public/assets/app.css"] != "body{color:red}" {
		t.Fatalf("unexpected published files: %#v", gotFiles)
	}
	if !strings.Contains(out, "published 2 files") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestSiteFilesAndDeleteFileUseExpectedAPI(t *testing.T) {
	var deletedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "alice" || password != "secret" {
			t.Fatalf("missing or wrong auth")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sites/demo/files":
			if r.URL.Query().Get("prefix") != "assets" {
				t.Fatalf("unexpected prefix: %q", r.URL.Query().Get("prefix"))
			}
			_ = json.NewEncoder(w).Encode([]siteFileInfo{{Path: "assets/app.css", Size: 15}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/sites/demo/files":
			deletedPath = r.URL.Query().Get("path")
			_ = json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	out, err := runCommand("site", "files", "demo", "assets", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "assets/app.css") || !strings.Contains(out, "15") {
		t.Fatalf("unexpected files output: %q", out)
	}

	out, err = runCommand("site", "delete-file", "demo", "assets/app.css", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if deletedPath != "assets/app.css" || !strings.Contains(out, "deleted assets/app.css") {
		t.Fatalf("delete file not observed: path=%q output=%q", deletedPath, out)
	}
}

func TestSiteListAndDeleteUseExpectedAPI(t *testing.T) {
	var deleted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "alice" || password != "secret" {
			t.Fatalf("missing or wrong auth")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sites":
			_ = json.NewEncoder(w).Encode([]siteMeta{{Slug: "demo", UpdatedAt: time.Date(2026, 6, 10, 21, 0, 0, 0, time.UTC)}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/sites/demo":
			deleted = true
			_ = json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	out, err := runCommand("site", "list", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "demo") || !strings.Contains(out, "/t/alice/s/demo/") {
		t.Fatalf("unexpected list output: %q", out)
	}

	out, err = runCommand("site", "delete", "demo", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted || !strings.Contains(out, "deleted demo") {
		t.Fatalf("delete not observed: deleted=%v output=%q", deleted, out)
	}
}

func TestSiteExampleListsAndPublishesBuiltInExamples(t *testing.T) {
	out, err := runCommand("site", "example")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"chat", "data", "library", "upload"} {
		if !strings.Contains(out, want) {
			t.Fatalf("example list missing %q: %q", want, out)
		}
	}

	var createdSlug string
	var publishedPath string
	var publishedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "alice" || password != "secret" {
			t.Fatalf("missing or wrong auth")
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/sites":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			createdSlug = body["slug"]
			_ = json.NewEncoder(w).Encode(siteMeta{Slug: createdSlug, UpdatedAt: time.Now().UTC()})
		case r.Method == http.MethodPut && r.URL.Path == "/api/sites/demo/files":
			publishedPath = r.URL.Query().Get("path")
			b, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			publishedBody = string(b)
			_ = json.NewEncoder(w).Encode(map[string]string{"path": publishedPath})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	out, err = runCommand("site", "example", "demo", "chat", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if createdSlug != "demo" || publishedPath != "index.html" || !strings.Contains(publishedBody, "Realtime chat") {
		t.Fatalf("example was not published: slug=%q path=%q body=%q", createdSlug, publishedPath, publishedBody)
	}
	if !strings.Contains(out, "published chat example") || !strings.Contains(out, server.URL+"/t/alice/s/demo/") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestMissingTenantCredentialsFailBeforeNetwork(t *testing.T) {
	_, err := runCommand("site", "list", "--server", "http://127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "missing tenant username") {
		t.Fatalf("expected missing tenant error, got %v", err)
	}
}

func TestHelpPrintsPlainBannerWhenCaptured(t *testing.T) {
	out, err := runCommand("--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "flink") || !strings.Contains(out, "live HTML/JS prototypes") {
		t.Fatalf("help should include banner, got %q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("captured help should not contain ANSI color: %q", out)
	}
}

func TestFlinkBannerColorRendering(t *testing.T) {
	out := banner.Render(true)
	if !strings.Contains(out, "\x1b[38;2;") || !strings.Contains(out, "flink") {
		t.Fatalf("color banner should contain ANSI and text, got %q", out)
	}
}

func runCommand(args ...string) (string, error) {
	cmd := NewRootCommandWithOptions(Options{ServerURL: "http://localhost:8080"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}
