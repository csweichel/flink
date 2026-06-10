package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"flink/server/api"
	"flink/server/storage"

	"github.com/gorilla/websocket"
)

const (
	testTenant   = "acme"
	testPassword = "secret"
)

func TestCreateEditHostAndDataAPI(t *testing.T) {
	a := testApp(t)
	postJSON(t, a, "/api/sites", map[string]string{"slug": "hello"})
	putJSON(t, a, "/api/sites/hello/files?path=index.html", map[string]string{"content": "<h1>live</h1><script src=\"/flink.js\"></script>"})

	res := request(t, a, http.MethodGet, "/s/hello/", nil, "")
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("live")) {
		t.Fatalf("site not served: %d %s", res.Code, res.Body.String())
	}

	putJSON(t, a, "/api/public/hello/data/note", map[string]any{"text": "saved"})
	res = request(t, a, http.MethodGet, "/api/public/hello/data/note", nil, "")
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("saved")) {
		t.Fatalf("data not saved: %d %s", res.Code, res.Body.String())
	}
}

func TestSubdomainHosting(t *testing.T) {
	a := testApp(t)
	postJSON(t, a, "/api/sites", map[string]string{"slug": "sub"})
	putJSON(t, a, "/api/sites/sub/files?path=index.html", map[string]string{"content": "<h1>subdomain</h1>"})

	req := httptest.NewRequest(http.MethodGet, "http://sub.quick.internal/", nil)
	req.SetBasicAuth(testTenant, testPassword)
	res := httptest.NewRecorder()
	a.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("subdomain")) {
		t.Fatalf("subdomain site not served: %d %s", res.Code, res.Body.String())
	}
}

func TestTenantRegistrationApprovalAndLogin(t *testing.T) {
	a := New(Config{DataDir: t.TempDir()})
	if err := a.Init(); err != nil {
		t.Fatal(err)
	}
	res := rawRequest(t, a, http.MethodGet, "/api/sites", nil, "")
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("API should require tenant auth, got %d", res.Code)
	}

	form := bytes.NewBufferString("username=newbie&password=secret")
	req := httptest.NewRequest(http.MethodPost, "/_flink/register", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	out := httptest.NewRecorder()
	a.ServeHTTP(out, req)
	if out.Code != http.StatusOK || !bytes.Contains(out.Body.Bytes(), []byte("pending approval")) {
		t.Fatalf("registration should be pending: %d %s", out.Code, out.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	req.SetBasicAuth("newbie", "secret")
	out = httptest.NewRecorder()
	a.ServeHTTP(out, req)
	if out.Code != http.StatusUnauthorized {
		t.Fatalf("pending tenant should not authenticate, got %d", out.Code)
	}

	if _, err := a.store.ApproveTenant("newbie"); err != nil {
		t.Fatal(err)
	}

	form = bytes.NewBufferString("username=newbie&password=secret")
	req = httptest.NewRequest(http.MethodPost, "/_flink/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	out = httptest.NewRecorder()
	a.ServeHTTP(out, req)
	if out.Code != http.StatusSeeOther || len(out.Result().Cookies()) == 0 {
		t.Fatalf("approved tenant login failed: %d %s", out.Code, out.Body.String())
	}
}

func TestTenantSessionCookieAuthenticatesDashboardAndAPI(t *testing.T) {
	a := testApp(t)
	session, err := a.store.CreateSession(testTenant, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	req.AddCookie(&http.Cookie{Name: "flink_session", Value: session.Token})
	res := httptest.NewRecorder()
	a.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("session cookie should authenticate API: %d %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/_flink", nil)
	req.AddCookie(&http.Cookie{Name: "flink_session", Value: session.Token})
	res = httptest.NewRecorder()
	a.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("session cookie should authenticate dashboard: %d %s", res.Code, res.Body.String())
	}
}

func TestDeniedTenantCannotAuthenticate(t *testing.T) {
	a := New(Config{DataDir: t.TempDir()})
	if err := a.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.CreateApprovedTenant("blocked", "secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.DenyTenant("blocked"); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
	req.SetBasicAuth("blocked", "secret")
	res := httptest.NewRecorder()
	a.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("denied tenant should not authenticate, got %d", res.Code)
	}
}

func TestTenantSiteSlugsAreIsolated(t *testing.T) {
	a := testApp(t)
	if _, err := a.store.CreateApprovedTenant("beta", testPassword); err != nil {
		t.Fatal(err)
	}

	postJSON(t, a, "/api/sites", map[string]string{"slug": "same"})
	putJSON(t, a, "/api/sites/same/files?path=index.html", map[string]string{"content": "acme site"})

	req := httptest.NewRequest(http.MethodPost, "/api/sites", bytes.NewReader([]byte(`{"slug":"same"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("beta", testPassword)
	res := httptest.NewRecorder()
	a.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("same slug should be allowed in another tenant: %d %s", res.Code, res.Body.String())
	}
	req = httptest.NewRequest(http.MethodPut, "/api/sites/same/files?path=index.html", bytes.NewReader([]byte(`{"content":"beta site"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("beta", testPassword)
	res = httptest.NewRecorder()
	a.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("beta write failed: %d %s", res.Code, res.Body.String())
	}

	res = request(t, a, http.MethodGet, "/t/acme/s/same/", nil, "")
	if res.Code != http.StatusOK || res.Body.String() != "acme site" {
		t.Fatalf("acme site leaked or failed: %d %q", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/t/beta/s/same/", nil)
	req.SetBasicAuth("beta", testPassword)
	res = httptest.NewRecorder()
	a.ServeHTTP(res, req)
	if res.Code != http.StatusOK || res.Body.String() != "beta site" {
		t.Fatalf("beta site leaked or failed: %d %q", res.Code, res.Body.String())
	}

	res = request(t, a, http.MethodGet, "/t/beta/s/same/", nil, "")
	if res.Code != http.StatusNotFound {
		t.Fatalf("authenticated tenant must not read another tenant canonical URL, got %d", res.Code)
	}
}

func TestDashboardServesEmbeddedFrontendBuild(t *testing.T) {
	a := testApp(t)
	res := request(t, a, http.MethodGet, "/_flink/", nil, "")
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte(`id="root"`)) {
		t.Fatalf("dashboard build not served: %d %s", res.Code, res.Body.String())
	}

	match := regexp.MustCompile(`src="/_flink/([^"]+\.js)"`).FindSubmatch(res.Body.Bytes())
	if len(match) != 2 {
		t.Fatalf("dashboard did not reference a built JS asset: %s", res.Body.String())
	}
	res = request(t, a, http.MethodGet, "/_flink/"+string(match[1]), nil, "")
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("React")) {
		t.Fatalf("dashboard JS asset not served: %d", res.Code)
	}

	res = request(t, a, http.MethodGet, "/flink.js", nil, "")
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("window.flink")) {
		t.Fatalf("client library not served: %d", res.Code)
	}
}

func TestUpload(t *testing.T) {
	a := testApp(t)
	postJSON(t, a, "/api/sites", map[string]string{"slug": "files"})
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("hello"))
	_ = mw.Close()
	res := request(t, a, http.MethodPost, "/api/public/files/uploads", &body, mw.FormDataContentType())
	if res.Code != http.StatusOK {
		t.Fatalf("upload failed: %d %s", res.Code, res.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	res = request(t, a, http.MethodGet, out["url"], nil, "")
	if res.Code != http.StatusOK || res.Body.String() != "hello" {
		t.Fatalf("uploaded file not served: %d %q", res.Code, res.Body.String())
	}
}

func TestDeleteSiteOverAPI(t *testing.T) {
	a := testApp(t)
	postJSON(t, a, "/api/sites", map[string]string{"slug": "delete-me"})

	res := request(t, a, http.MethodDelete, "/api/sites/delete-me", nil, "")
	if res.Code != http.StatusOK {
		t.Fatalf("delete failed: %d %s", res.Code, res.Body.String())
	}
	res = request(t, a, http.MethodGet, "/s/delete-me/", nil, "")
	if res.Code != http.StatusNotFound {
		t.Fatalf("site should be gone, got %d", res.Code)
	}
}

func TestAIEndpointWithoutKeyIsStable(t *testing.T) {
	a := testApp(t)
	postJSON(t, a, "/api/sites", map[string]string{"slug": "ai"})

	res := request(t, a, http.MethodPost, "/api/public/ai/ai", bytes.NewReader([]byte(`{"prompt":"hello"}`)), "application/json")
	if res.Code != http.StatusOK {
		t.Fatalf("AI endpoint should return stable unconfigured response: %d %s", res.Code, res.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["configured"] != false || out["text"] == "" {
		t.Fatalf("unexpected unconfigured AI response: %#v", out)
	}
}

func TestAIEndpointCallsOpenAICompatibleResponsesAPI(t *testing.T) {
	var gotAuth string
	var gotPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected AI upstream path: %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"mock-model","output":[{"content":[{"type":"output_text","text":"hello from ai"}]}]}`)
	}))
	defer upstream.Close()

	a := testAppWithConfig(t, Config{
		DataDir:  t.TempDir(),
		BaseHost: "quick.internal",
		AI: api.AIConfig{
			APIKey:  "test-key",
			BaseURL: upstream.URL,
			Model:   "mock-model",
		},
	})
	postJSON(t, a, "/api/sites", map[string]string{"slug": "ai"})

	res := request(t, a, http.MethodPost, "/api/public/ai/ai", bytes.NewReader([]byte(`{"prompt":"hello","instructions":"be brief","maxOutputTokens":32}`)), "application/json")
	if res.Code != http.StatusOK {
		t.Fatalf("AI endpoint failed: %d %s", res.Code, res.Body.String())
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("unexpected authorization header: %q", gotAuth)
	}
	if gotPayload["model"] != "mock-model" || gotPayload["input"] != "hello" || gotPayload["instructions"] != "be brief" || gotPayload["max_output_tokens"] != float64(32) || gotPayload["store"] != false {
		t.Fatalf("unexpected AI payload: %#v", gotPayload)
	}
	var out map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["text"] != "hello from ai" || out["model"] != "mock-model" || out["configured"] != true {
		t.Fatalf("unexpected AI response: %#v", out)
	}
}

func TestWebSocketBroadcast(t *testing.T) {
	a := testApp(t)
	postJSON(t, a, "/api/sites", map[string]string{"slug": "chat"})
	srv := httptest.NewServer(a)
	defer srv.Close()
	wsURL := "ws" + srv.URL[len("http"):] + "/ws/chat/main"
	header := http.Header{"Authorization": []string{basicAuth(testTenant, testPassword)}}
	c1, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	c2, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	deadline := time.Now().Add(2 * time.Second)
	_ = c1.SetReadDeadline(deadline)
	_ = c2.SetReadDeadline(deadline)
	if err := c2.WriteMessage(websocket.TextMessage, []byte(`{"ready":true}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c1.ReadMessage(); err != nil {
		t.Fatal(err)
	}

	if err := c1.WriteMessage(websocket.TextMessage, []byte(`{"text":"hi"}`)); err != nil {
		t.Fatal(err)
	}
	_, msg, err := c2.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(msg) != `{"text":"hi"}` {
		t.Fatalf("unexpected ws message: %s", msg)
	}
}

func TestCLIWriteUsesSafePaths(t *testing.T) {
	dir := t.TempDir()
	backend := storage.NewFileBackend(dir)
	if err := backend.Init(nil); err != nil {
		t.Fatal(err)
	}
	store := api.NewStore(backend, "")
	if _, err := store.CreateApprovedTenant(testTenant, testPassword); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSite(testTenant, "safe", ""); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteSiteFile(testTenant, "safe", "../escape.txt", []byte("no")); err == nil {
		t.Fatal("expected path traversal error")
	}
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("escape file exists or stat failed unexpectedly: %v", err)
	}
}

func TestBboltStorageDriver(t *testing.T) {
	a := New(Config{DataDir: t.TempDir(), StorageDriver: "bbolt"})
	if err := a.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.store.CreateApprovedTenant(testTenant, testPassword); err != nil {
		t.Fatal(err)
	}
	postJSON(t, a, "/api/sites", map[string]string{"slug": "bolt"})
	putJSON(t, a, "/api/sites/bolt/files?path=index.html", map[string]string{"content": "<h1>bbolt</h1>"})

	res := request(t, a, http.MethodGet, "/s/bolt/", nil, "")
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("bbolt")) {
		t.Fatalf("bbolt-backed site not served: %d %s", res.Code, res.Body.String())
	}
	putJSON(t, a, "/api/public/bolt/data/state", map[string]any{"driver": "bbolt"})
	res = request(t, a, http.MethodGet, "/api/public/bolt/data/state", nil, "")
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte("bbolt")) {
		t.Fatalf("bbolt-backed data not served: %d %s", res.Code, res.Body.String())
	}
}

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

func request(t *testing.T, h http.Handler, method, url string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.SetBasicAuth(testTenant, testPassword)
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
