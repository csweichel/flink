package cmd

import (
	"bytes"
	"encoding/base64"
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

func TestPublishCreatesSitePublishesDirectoryAndRecordsVersion(t *testing.T) {
	dir := t.TempDir()
	chdir(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<script src=\"/flink.js\"></script>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.css"), []byte("body{color:red}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "ignored"), []byte("no"), 0644); err != nil {
		t.Fatal(err)
	}

	gotFiles := map[string]string{}
	var recorded publishRecord
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "alice" || password != "secret" {
			t.Fatalf("missing or wrong auth")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sites/demo":
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/api/sites":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(siteMeta{Slug: body["slug"], Auth: siteAuthPolicy{Mode: "owner"}, UpdatedAt: time.Now().UTC()})
		case r.Method == http.MethodGet && r.URL.Path == "/api/sites/demo/files":
			_ = json.NewEncoder(w).Encode([]siteFileInfo{{Path: "old.html", Size: 3}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/sites/demo/files":
			if r.URL.Query().Get("path") != "old.html" {
				t.Fatalf("unexpected deleted path %q", r.URL.Query().Get("path"))
			}
			_ = json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
		case r.Method == http.MethodPut && r.URL.Path == "/api/sites/demo/files":
			b, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			gotFiles[r.URL.Query().Get("path")] = string(b)
			_ = json.NewEncoder(w).Encode(map[string]string{"path": r.URL.Query().Get("path")})
		case r.Method == http.MethodPost && r.URL.Path == "/api/sites/demo/publishes":
			if err := json.NewDecoder(r.Body).Decode(&recorded); err != nil {
				t.Fatal(err)
			}
			recorded.ID = "v1"
			_ = json.NewEncoder(w).Encode(recorded)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	out, err := runCommand("publish", dir, "--site", "demo", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if gotFiles["index.html"] == "" || gotFiles["assets/app.css"] != "body{color:red}" {
		t.Fatalf("unexpected files: %#v", gotFiles)
	}
	if recorded.FileCount != 2 || recorded.TotalBytes == 0 {
		t.Fatalf("publish record not populated: %#v", recorded)
	}
	for _, want := range []string{"Site created and published", "Target", "Site         demo", "Result", "Uploaded     2 files", "Removed      1 stale files", "Links", server.URL + "/t/alice/s/demo/"} {
		if !strings.Contains(out, want) {
			t.Fatalf("publish output missing %q: %q", want, out)
		}
	}
}

func TestInitWritesTemplateAndProjectConfig(t *testing.T) {
	dir := t.TempDir()
	out, err := runCommand("init", "todo", dir, "--site", "tasks", "--server", "https://flink.example", "--tenant", "alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Template created", "Project", "Template", "todo", "Site         tasks", "Next", "Publish      flink publish"} {
		if !strings.Contains(out, want) {
			t.Fatalf("init output missing %q: %q", want, out)
		}
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "/flink.js") || !strings.Contains(string(index), "flink.set") {
		t.Fatalf("template missing SDK usage: %s", string(index))
	}
	config := readProjectConfig(dir)
	if config.Site != "tasks" || config.Server != "https://flink.example" || config.Tenant != "alice" {
		t.Fatalf("unexpected project config: %#v", config)
	}
}

func TestTopLevelAuthAndListUseExpectedAPIs(t *testing.T) {
	var gotPolicy siteAuthPolicy
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "alice" || password != "secret" {
			t.Fatalf("missing or wrong auth")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sites":
			_ = json.NewEncoder(w).Encode([]siteMeta{{Slug: "demo", Auth: siteAuthPolicy{Mode: "owner"}, UpdatedAt: time.Date(2026, 6, 10, 21, 0, 0, 0, time.UTC)}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/sites/demo/auth":
			if err := json.NewDecoder(r.Body).Decode(&gotPolicy); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(gotPolicy)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	out, err := runCommand("list", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Sites", "demo", "Access", "Updated", "/t/alice/s/demo/"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list output missing %q: %q", want, out)
		}
	}
	out, err = runCommand("auth", "demo", "tenants", "bob", "alice", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if gotPolicy.Mode != "tenants" || strings.Join(gotPolicy.Tenants, ",") != "alice,bob" {
		t.Fatalf("unexpected policy: %#v", gotPolicy)
	}
	for _, want := range []string{"Access policy", "Target", "Site         demo", "Access", "Mode         tenants (alice, bob)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("auth output missing %q: %q", want, out)
		}
	}
}

func TestAPICommandsUseSiteAPIEndpoints(t *testing.T) {
	uploadPath := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(uploadPath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	var gotData map[string]any
	var gotAI aiRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "alice" || password != "secret" {
			t.Fatalf("missing or wrong auth")
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/sites/demo/data/note":
			if err := json.NewDecoder(r.Body).Decode(&gotData); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(gotData)
		case r.Method == http.MethodGet && r.URL.Path == "/api/sites/demo/data/note":
			_ = json.NewEncoder(w).Encode(map[string]any{"text": "saved"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/sites/demo/files" && r.URL.Query().Get("path") == "index.html":
			_ = json.NewEncoder(w).Encode(siteFile{Path: "index.html", Content: "<h1>demo</h1>"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/sites/demo/uploads":
			if err := r.ParseMultipartForm(32 << 20); err != nil {
				t.Fatal(err)
			}
			f, header, err := r.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			b, err := io.ReadAll(f)
			if err != nil {
				t.Fatal(err)
			}
			if header.Filename != "hello.txt" || string(b) != "hello" {
				t.Fatalf("unexpected upload filename=%q body=%q", header.Filename, string(b))
			}
			_ = json.NewEncoder(w).Encode(uploadResult{Name: "hello.txt", URL: "/uploads/alice/demo/stored.txt"})
		case r.Method == http.MethodGet && r.URL.Path == "/uploads/alice/demo/stored.txt":
			_, _ = w.Write([]byte("hello"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/sites/demo/ai":
			if err := json.NewDecoder(r.Body).Decode(&gotAI); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(aiResponse{Text: "idea", Model: "mock", Configured: true})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	out, err := runCommand("api", "data", "set", "demo", "note", `{"text":"saved"}`, "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if gotData["text"] != "saved" || !strings.Contains(out, `"text": "saved"`) {
		t.Fatalf("unexpected data set: got=%#v out=%q", gotData, out)
	}

	out, err = runCommand("api", "data", "get", "demo", "note", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"text": "saved"`) {
		t.Fatalf("unexpected data get output: %q", out)
	}

	out, err = runCommand("api", "files", "read", "demo", "index.html", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if out != "<h1>demo</h1>" {
		t.Fatalf("unexpected file read output: %q", out)
	}

	out, err = runCommand("api", "uploads", "upload", "demo", uploadPath, "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `/uploads/alice/demo/stored.txt`) {
		t.Fatalf("unexpected upload output: %q", out)
	}

	out, err = runCommand("api", "uploads", "fetch", "demo", "/uploads/alice/demo/stored.txt", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello" {
		t.Fatalf("unexpected upload fetch output: %q", out)
	}

	out, err = runCommand("api", "ai", "demo", "make it useful", "--instructions", "be concise", "--model", "mock", "--max-output-tokens", "32", "--json", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if gotAI.Prompt != "make it useful" || gotAI.Instructions != "be concise" || gotAI.Model != "mock" || gotAI.MaxOutputTokens != 32 || !strings.Contains(out, `"text": "idea"`) {
		t.Fatalf("unexpected ai call: got=%#v out=%q", gotAI, out)
	}
}

func TestAgentCommandsEnableStatusAndListenToStoredMessage(t *testing.T) {
	var enabled bool
	var deletedID string
	var gotResponse string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "alice" || password != "secret" {
			t.Fatalf("missing or wrong auth")
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/sites/demo/agent":
			var body map[string]bool
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			enabled = body["enabled"]
			_ = json.NewEncoder(w).Encode(agentStatus{Enabled: enabled, Listening: false, Pending: 1})
		case r.Method == http.MethodGet && r.URL.Path == "/api/sites/demo/agent":
			_ = json.NewEncoder(w).Encode(agentStatus{Enabled: enabled, Listening: false, Pending: 1})
		case r.Method == http.MethodGet && r.URL.Path == "/api/sites/demo/agent/messages":
			_ = json.NewEncoder(w).Encode([]agentMessage{{
				ID:     "m1",
				Tenant: "alice",
				Site:   "demo",
				Text:   "make the button blue",
				Sender: "alice",
				Screenshot: &agentScreenshot{
					Name:    "page.png",
					Type:    "image/png",
					DataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png")),
				},
				CreatedAt: time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC),
			}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/sites/demo/agent/messages/m1":
			deletedID = "m1"
			_ = json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
		case r.Method == http.MethodPost && r.URL.Path == "/api/sites/demo/agent/responses":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			gotResponse = body["text"]
			_ = json.NewEncoder(w).Encode(agentResponse{ID: "r1", Tenant: "alice", Site: "demo", Text: gotResponse, CreatedAt: time.Date(2026, 6, 11, 12, 1, 0, 0, time.UTC)})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	out, err := runCommand("agent", "enable", "demo", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !enabled || !strings.Contains(out, "Agent messages") || !strings.Contains(out, "Enabled      true") {
		t.Fatalf("unexpected enable output: enabled=%v out=%q", enabled, out)
	}

	out, err = runCommand("agent", "status", "demo", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Pending      1") {
		t.Fatalf("unexpected status output: %q", out)
	}

	out, err = runCommand("agent", "listen", "demo", "--once", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Agent message", "make the button blue", "Screenshot", "flink agent respond demo <message>", "flink publish <path> --site demo", "flink agent listen demo", server.URL + "/t/alice/s/demo/"} {
		if !strings.Contains(out, want) {
			t.Fatalf("listen output missing %q: %q", want, out)
		}
	}
	if deletedID != "m1" {
		t.Fatalf("stored message was not acknowledged: %q", deletedID)
	}

	out, err = runCommand("agent", "respond", "demo", "Done", "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if gotResponse != "Done" || !strings.Contains(out, "Agent response") || !strings.Contains(out, "Done") {
		t.Fatalf("unexpected respond result: got=%q out=%q", gotResponse, out)
	}
}

func TestSnapshotExportsFilesAndManifest(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); !ok {
			t.Fatalf("missing auth")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sites/demo/files" && !r.URL.Query().Has("path"):
			_ = json.NewEncoder(w).Encode([]siteFileInfo{{Path: "index.html", Size: 11}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/sites/demo/files" && r.URL.Query().Get("path") == "index.html":
			_ = json.NewEncoder(w).Encode(map[string]string{"path": "index.html", "content": "<h1>x</h1>"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	out, err := runCommand("snapshot", "demo", dir, "--server", server.URL, "--tenant", "alice", "--password", "secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Snapshot written", "Source", "Site         demo", "Output", "Files        1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("snapshot output missing %q: %q", want, out)
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "index.html")); err != nil || string(b) != "<h1>x</h1>" {
		t.Fatalf("snapshot file missing: %q %v", string(b), err)
	}
	if _, err := os.Stat(filepath.Join(dir, "flink-snapshot.json")); err != nil {
		t.Fatal(err)
	}
}

func TestOldSiteNamespaceIsGone(t *testing.T) {
	_, err := runCommand("site", "list")
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected old site namespace to be gone, got %v", err)
	}
}

func TestMissingTenantCredentialsFailBeforeNetwork(t *testing.T) {
	_, err := runCommand("list", "--server", "http://127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "missing tenant username") {
		t.Fatalf("expected missing tenant error, got %v", err)
	}
	if !strings.Contains(err.Error(), "Set FLINK_SERVER=http://127.0.0.1:1, FLINK_TENANT, and FLINK_PASSWORD") {
		t.Fatalf("missing tenant error should include config hint, got %v", err)
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

func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	})
}
