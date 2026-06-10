package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotContent = body["content"]
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

func TestMissingTenantCredentialsFailBeforeNetwork(t *testing.T) {
	_, err := runCommand("site", "list", "--server", "http://127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "missing tenant username") {
		t.Fatalf("expected missing tenant error, got %v", err)
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
