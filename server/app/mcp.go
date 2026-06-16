package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"

	mcp "github.com/metoro-io/mcp-golang"
	"github.com/metoro-io/mcp-golang/transport"

	"github.com/csweichel/flink/server/api"
)

type mcpTenantContextKey struct{}
type mcpOriginContextKey struct{}

func (a *App) handleMCP(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromContext(r.Context())
	ctx := context.WithValue(r.Context(), mcpTenantContextKey{}, tenant.Username)
	ctx = context.WithValue(ctx, mcpOriginContextKey{}, requestOrigin(r))
	a.mcp.ServeHTTP(w, r.WithContext(ctx))
}

func (a *App) newMCPHandler() http.Handler {
	mcpTransport := newFlinkMCPTransport()
	server := mcp.NewServer(
		mcpTransport,
		mcp.WithName("flink"),
		mcp.WithVersion("dev"),
		mcp.WithInstructions("Use these tools to publish and manage sites for the authenticated Flink tenant."),
	)
	mustRegisterMCPTool(server, "flink_list_sites", "List sites owned by the authenticated tenant.", a.mcpListSites)
	mustRegisterMCPTool(server, "flink_get_site", "Get site metadata, files, JSON data keys, and uploads.", a.mcpGetSite)
	mustRegisterMCPTool(server, "flink_publish_site", "Create or replace a site from inline file contents and record a publish version.", a.mcpPublishSite)
	mustRegisterMCPTool(server, "flink_read_file", "Read a hosted site file as text.", a.mcpReadFile)
	mustRegisterMCPTool(server, "flink_write_file", "Write one hosted site file.", a.mcpWriteFile)
	mustRegisterMCPTool(server, "flink_delete_file", "Delete one hosted site file.", a.mcpDeleteFile)
	mustRegisterMCPTool(server, "flink_set_site_auth", "Configure site access policy: owner, none, or tenants.", a.mcpSetSiteAuth)
	mustRegisterMCPTool(server, "flink_get_site_data", "Read all JSON storage for a site, or one key.", a.mcpGetSiteData)
	mustRegisterMCPTool(server, "flink_set_site_data", "Set one JSON storage key for a site.", a.mcpSetSiteData)
	mustRegisterMCPTool(server, "flink_delete_site_data", "Delete one JSON storage key for a site.", a.mcpDeleteSiteData)
	mustRegisterMCPTool(server, "flink_list_publishes", "List publish versions for a site.", a.mcpListPublishes)
	mustRegisterMCPTool(server, "flink_rollback_site", "Restore a previous publish version. Omit version to restore the previous publish.", a.mcpRollbackSite)
	if err := server.Serve(); err != nil {
		panic(err)
	}
	return mcpTransport
}

func mustRegisterMCPTool(server *mcp.Server, name, description string, handler any) {
	if err := server.RegisterTool(name, description, handler); err != nil {
		panic(err)
	}
}

func mcpContextTenant(ctx context.Context) string {
	if tenant, ok := ctx.Value(mcpTenantContextKey{}).(string); ok {
		return tenant
	}
	return ""
}

func mcpContextOrigin(ctx context.Context) string {
	if origin, ok := ctx.Value(mcpOriginContextKey{}).(string); ok {
		return origin
	}
	return ""
}

func (a *App) mcpListSites(ctx context.Context, args mcpNoArgs) (*mcp.ToolResponse, error) {
	return mcpJSON(a.store.ListSites(mcpContextTenant(ctx)))
}

func (a *App) mcpGetSite(ctx context.Context, args mcpSiteArgs) (*mcp.ToolResponse, error) {
	return mcpJSON(a.store.ReadSiteDetails(mcpContextTenant(ctx), args.Site))
}

func (a *App) mcpPublishSite(ctx context.Context, args mcpPublishSiteArgs) (*mcp.ToolResponse, error) {
	tenant := mcpContextTenant(ctx)
	if len(args.Files) == 0 {
		return nil, fmt.Errorf("files are required")
	}
	meta, err := a.store.CreateSite(tenant, args.Site, args.Title)
	if err != nil {
		return nil, err
	}
	if args.Auth != nil {
		meta, err = a.store.UpdateSiteAuth(tenant, args.Site, *args.Auth)
		if err != nil {
			return nil, err
		}
	}
	deleteStale := true
	if args.DeleteStale != nil {
		deleteStale = *args.DeleteStale
	}
	next := map[string]bool{}
	manifest := make([]api.PublishFileInfo, 0, len(args.Files))
	totalBytes := 0
	for _, file := range args.Files {
		content, err := file.bytes()
		if err != nil {
			return nil, err
		}
		p, err := api.CleanPath(file.Path)
		if err != nil {
			return nil, err
		}
		next[p] = true
		if err := a.store.WriteSiteFile(tenant, args.Site, p, content); err != nil {
			return nil, err
		}
		sum := sha256.Sum256(content)
		manifest = append(manifest, api.PublishFileInfo{Path: p, Size: len(content), Hash: hex.EncodeToString(sum[:])})
		totalBytes += len(content)
	}
	deleted := 0
	if deleteStale {
		existing, err := a.store.ListSiteFiles(tenant, args.Site, "")
		if err != nil {
			return nil, err
		}
		for _, file := range existing {
			if !next[file.Path] {
				if err := a.store.DeleteSiteFile(tenant, args.Site, file.Path); err != nil {
					return nil, err
				}
				deleted++
			}
		}
	}
	sort.Slice(manifest, func(i, j int) bool { return manifest[i].Path < manifest[j].Path })
	record, err := a.store.RecordPublish(tenant, args.Site, api.PublishRecord{
		Source:     strings.TrimSpace(args.Source),
		FileCount:  len(manifest),
		TotalBytes: totalBytes,
		Files:      manifest,
		Auth:       meta.Auth,
	})
	if err != nil {
		return nil, err
	}
	return mcpJSON(map[string]any{
		"site":       args.Site,
		"url":        a.siteURL(mcpContextOrigin(ctx), tenant, args.Site),
		"published":  len(manifest),
		"deleted":    deleted,
		"totalBytes": totalBytes,
		"auth":       meta.Auth,
		"publish":    record,
	}, nil)
}

func (a *App) mcpReadFile(ctx context.Context, args mcpFilePathArgs) (*mcp.ToolResponse, error) {
	b, err := a.store.ReadSiteFile(mcpContextTenant(ctx), args.Site, args.Path)
	if err != nil {
		return nil, err
	}
	return mcpJSON(map[string]any{"site": args.Site, "path": args.Path, "content": string(b)}, nil)
}

func (a *App) mcpWriteFile(ctx context.Context, args mcpWriteFileArgs) (*mcp.ToolResponse, error) {
	content, err := args.bytes()
	if err != nil {
		return nil, err
	}
	if err := a.store.WriteSiteFile(mcpContextTenant(ctx), args.Site, args.Path, content); err != nil {
		return nil, err
	}
	return mcpJSON(map[string]any{"site": args.Site, "path": args.Path, "size": len(content)}, nil)
}

func (a *App) mcpDeleteFile(ctx context.Context, args mcpFilePathArgs) (*mcp.ToolResponse, error) {
	if err := a.store.DeleteSiteFile(mcpContextTenant(ctx), args.Site, args.Path); err != nil {
		return nil, err
	}
	return mcpJSON(map[string]any{"site": args.Site, "path": args.Path, "deleted": true}, nil)
}

func (a *App) mcpSetSiteAuth(ctx context.Context, args mcpSiteAuthArgs) (*mcp.ToolResponse, error) {
	meta, err := a.store.UpdateSiteAuth(mcpContextTenant(ctx), args.Site, api.SiteAuthPolicy{Mode: args.Mode, Tenants: args.Tenants})
	if err != nil {
		return nil, err
	}
	return mcpJSON(meta.Auth, nil)
}

func (a *App) mcpGetSiteData(ctx context.Context, args mcpSiteDataGetArgs) (*mcp.ToolResponse, error) {
	data, err := a.store.ReadData(mcpContextTenant(ctx), args.Site)
	if err != nil {
		return nil, err
	}
	if args.Key == "" {
		return mcpJSON(data, nil)
	}
	value, ok := data[strings.Trim(args.Key, "/")]
	if !ok {
		return nil, fmt.Errorf("key not found")
	}
	return mcpJSON(value, nil)
}

func (a *App) mcpSetSiteData(ctx context.Context, args mcpSiteDataSetArgs) (*mcp.ToolResponse, error) {
	key := strings.Trim(args.Key, "/")
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}
	data, err := a.store.ReadData(mcpContextTenant(ctx), args.Site)
	if err != nil {
		return nil, err
	}
	data[key] = args.Value
	if err := a.store.WriteData(mcpContextTenant(ctx), args.Site, data); err != nil {
		return nil, err
	}
	return mcpJSON(map[string]any{"site": args.Site, "key": key, "value": args.Value}, nil)
}

func (a *App) mcpDeleteSiteData(ctx context.Context, args mcpSiteDataDeleteArgs) (*mcp.ToolResponse, error) {
	key := strings.Trim(args.Key, "/")
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}
	data, err := a.store.ReadData(mcpContextTenant(ctx), args.Site)
	if err != nil {
		return nil, err
	}
	delete(data, key)
	if err := a.store.WriteData(mcpContextTenant(ctx), args.Site, data); err != nil {
		return nil, err
	}
	return mcpJSON(map[string]any{"site": args.Site, "key": key, "deleted": true}, nil)
}

func (a *App) mcpListPublishes(ctx context.Context, args mcpSiteArgs) (*mcp.ToolResponse, error) {
	return mcpJSON(a.store.ListPublishes(mcpContextTenant(ctx), args.Site))
}

func (a *App) mcpRollbackSite(ctx context.Context, args mcpRollbackSiteArgs) (*mcp.ToolResponse, error) {
	tenant := mcpContextTenant(ctx)
	record, err := a.store.RollbackPublish(tenant, args.Site, args.Version)
	if err != nil {
		return nil, err
	}
	return mcpJSON(map[string]any{
		"site":    args.Site,
		"url":     a.siteURL(mcpContextOrigin(ctx), tenant, args.Site),
		"publish": record,
	}, nil)
}

func mcpJSON(v any, err error) (*mcp.ToolResponse, error) {
	if err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResponse(mcp.NewTextContent(string(b))), nil
}

type mcpNoArgs struct{}

type mcpSiteArgs struct {
	Site string `json:"site" jsonschema:"required,description=Site slug."`
}

type mcpFilePathArgs struct {
	Site string `json:"site" jsonschema:"required,description=Site slug."`
	Path string `json:"path" jsonschema:"required,description=Hosted file path."`
}

type mcpWriteFileArgs struct {
	Site          string `json:"site" jsonschema:"required,description=Site slug."`
	Path          string `json:"path" jsonschema:"required,description=Hosted file path."`
	Content       string `json:"content,omitempty" jsonschema:"description=Text content."`
	ContentBase64 string `json:"contentBase64,omitempty" jsonschema:"description=Base64 content for binary files."`
}

func (f mcpWriteFileArgs) bytes() ([]byte, error) {
	if f.ContentBase64 != "" {
		return base64.StdEncoding.DecodeString(f.ContentBase64)
	}
	return []byte(f.Content), nil
}

type mcpPublishFileArgs struct {
	Path          string `json:"path" jsonschema:"required,description=Hosted file path."`
	Content       string `json:"content,omitempty" jsonschema:"description=Text content."`
	ContentBase64 string `json:"contentBase64,omitempty" jsonschema:"description=Base64 content for binary files."`
}

func (f mcpPublishFileArgs) bytes() ([]byte, error) {
	if f.ContentBase64 != "" {
		return base64.StdEncoding.DecodeString(f.ContentBase64)
	}
	return []byte(f.Content), nil
}

type mcpPublishSiteArgs struct {
	Site        string               `json:"site" jsonschema:"required,description=Site slug."`
	Title       string               `json:"title,omitempty" jsonschema:"description=Optional site title."`
	Files       []mcpPublishFileArgs `json:"files" jsonschema:"required,description=Files to publish. Use content for text or contentBase64 for binary data."`
	Auth        *api.SiteAuthPolicy  `json:"auth,omitempty" jsonschema:"description=Optional site access policy."`
	DeleteStale *bool                `json:"deleteStale,omitempty" jsonschema:"description=Delete hosted files not included in this publish. Defaults to true."`
	Source      string               `json:"source,omitempty" jsonschema:"description=Optional publish source label."`
}

type mcpSiteAuthArgs struct {
	Site    string   `json:"site" jsonschema:"required,description=Site slug."`
	Mode    string   `json:"mode" jsonschema:"required,description=Access mode: owner none or tenants."`
	Tenants []string `json:"tenants,omitempty" jsonschema:"description=Optional tenant allow-list for tenants mode."`
}

type mcpSiteDataGetArgs struct {
	Site string `json:"site" jsonschema:"required,description=Site slug."`
	Key  string `json:"key,omitempty" jsonschema:"description=Optional storage key."`
}

type mcpSiteDataSetArgs struct {
	Site  string `json:"site" jsonschema:"required,description=Site slug."`
	Key   string `json:"key" jsonschema:"required,description=Storage key."`
	Value any    `json:"value" jsonschema:"required,description=JSON value."`
}

type mcpSiteDataDeleteArgs struct {
	Site string `json:"site" jsonschema:"required,description=Site slug."`
	Key  string `json:"key" jsonschema:"required,description=Storage key."`
}

type mcpRollbackSiteArgs struct {
	Site    string `json:"site" jsonschema:"required,description=Site slug."`
	Version string `json:"version,omitempty" jsonschema:"description=Optional publish version ID."`
}

type flinkMCPTransport struct {
	messageHandler func(context.Context, *transport.BaseJsonRpcMessage)
	errorHandler   func(error)
	closeHandler   func()
	mu             sync.Mutex
	responses      map[transport.RequestId]chan *transport.BaseJsonRpcMessage
}

func newFlinkMCPTransport() *flinkMCPTransport {
	return &flinkMCPTransport{responses: map[transport.RequestId]chan *transport.BaseJsonRpcMessage{}}
}

func (t *flinkMCPTransport) Start(ctx context.Context) error { return nil }

func (t *flinkMCPTransport) Close() error {
	if t.closeHandler != nil {
		t.closeHandler()
	}
	return nil
}

func (t *flinkMCPTransport) SetCloseHandler(handler func()) { t.closeHandler = handler }
func (t *flinkMCPTransport) SetErrorHandler(handler func(error)) {
	t.errorHandler = handler
}
func (t *flinkMCPTransport) SetMessageHandler(handler func(context.Context, *transport.BaseJsonRpcMessage)) {
	t.messageHandler = handler
}

func (t *flinkMCPTransport) Send(ctx context.Context, message *transport.BaseJsonRpcMessage) error {
	if message == nil || message.JsonRpcResponse == nil {
		return nil
	}
	t.mu.Lock()
	ch := t.responses[message.JsonRpcResponse.Id]
	t.mu.Unlock()
	if ch == nil {
		return fmt.Errorf("no response channel for request id %d", message.JsonRpcResponse.Id)
	}
	ch <- message
	return nil
}

func (t *flinkMCPTransport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST method is supported", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var request transport.BaseJSONRPCRequest
	if err := json.Unmarshal(body, &request); err == nil {
		response, err := t.handleRequest(r.Context(), &request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	var notification transport.BaseJSONRPCNotification
	if err := json.Unmarshal(body, &notification); err == nil {
		if t.messageHandler != nil {
			t.messageHandler(r.Context(), transport.NewBaseMessageNotification(&notification))
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	http.Error(w, "invalid JSON-RPC message", http.StatusBadRequest)
}

func (t *flinkMCPTransport) handleRequest(ctx context.Context, request *transport.BaseJSONRPCRequest) (*transport.BaseJsonRpcMessage, error) {
	ch := make(chan *transport.BaseJsonRpcMessage, 1)
	t.mu.Lock()
	if _, exists := t.responses[request.Id]; exists {
		t.mu.Unlock()
		return nil, fmt.Errorf("duplicate request id %d", request.Id)
	}
	t.responses[request.Id] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.responses, request.Id)
		t.mu.Unlock()
	}()
	if t.messageHandler == nil {
		return nil, fmt.Errorf("mcp message handler is not configured")
	}
	t.messageHandler(ctx, transport.NewBaseMessageRequest(request))
	select {
	case response := <-ch:
		return response, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
