package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/csweichel/flink/server/storage"
)

var ErrNotFound = storage.ErrNotFound

const (
	TenantPending  = "pending"
	TenantApproved = "approved"
	TenantDenied   = "denied"

	SiteAuthNone    = "none"
	SiteAuthOwner   = "owner"
	SiteAuthTenants = "tenants"
)

type Store struct {
	backend             storage.Backend
	defaultIndex        string
	defaultSiteAuthMode string
}

type SiteMeta struct {
	Slug      string         `json:"slug"`
	Title     string         `json:"title"`
	Auth      SiteAuthPolicy `json:"auth"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type SiteAuthPolicy struct {
	Mode    string   `json:"mode"`
	Tenants []string `json:"tenants,omitempty"`
}

type TenantMeta struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"passwordHash,omitempty"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type PublicTenant struct {
	Username  string    `json:"username"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Session struct {
	Token     string    `json:"token"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Upload struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

type UploadInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int    `json:"size"`
}

type SiteFileInfo struct {
	Path string `json:"path"`
	Size int    `json:"size"`
}

func NewStore(backend storage.Backend, defaultIndex string) *Store {
	return &Store{backend: backend, defaultIndex: defaultIndex, defaultSiteAuthMode: SiteAuthOwner}
}

func (s *Store) SetDefaultSiteAuthMode(mode string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = SiteAuthOwner
	}
	switch mode {
	case SiteAuthOwner, SiteAuthNone:
		s.defaultSiteAuthMode = mode
		return nil
	case SiteAuthTenants:
		s.defaultSiteAuthMode = SiteAuthTenants
		return nil
	default:
		return fmt.Errorf("invalid default site auth mode %q", mode)
	}
}

func (s *Store) RegisterTenant(username, password string) (PublicTenant, error) {
	return s.createTenant(username, password, TenantPending, false)
}

func (s *Store) RegisterApprovedTenant(username, password string) (PublicTenant, error) {
	return s.createTenant(username, password, TenantApproved, false)
}

func (s *Store) CreateApprovedTenant(username, password string) (PublicTenant, error) {
	return s.createTenant(username, password, TenantApproved, true)
}

func (s *Store) createTenant(username, password, status string, overwrite bool) (PublicTenant, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !ValidSlug(username) {
		return PublicTenant{}, fmt.Errorf("invalid username %q: use lowercase letters, numbers, and dashes", username)
	}
	if strings.TrimSpace(password) == "" {
		return PublicTenant{}, fmt.Errorf("password is required")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return PublicTenant{}, err
	}
	now := time.Now().UTC()
	meta := TenantMeta{
		Username:     username,
		PasswordHash: hash,
		Status:       status,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if existing, err := s.ReadTenant(username); err == nil && overwrite {
		meta.CreatedAt = existing.CreatedAt
	} else if err == nil {
		return PublicTenant{}, fmt.Errorf("tenant %q already exists", username)
	} else if !errors.Is(err, ErrNotFound) {
		return PublicTenant{}, err
	}
	if err := s.writeTenant(meta); err != nil {
		return PublicTenant{}, err
	}
	return meta.Public(), nil
}

func (s *Store) ResetTenantPassword(username, password string) (PublicTenant, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if strings.TrimSpace(password) == "" {
		return PublicTenant{}, fmt.Errorf("password is required")
	}
	meta, err := s.ReadTenant(username)
	if err != nil {
		return PublicTenant{}, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return PublicTenant{}, err
	}
	meta.PasswordHash = hash
	meta.UpdatedAt = time.Now().UTC()
	if err := s.writeTenant(meta); err != nil {
		return PublicTenant{}, err
	}
	return meta.Public(), nil
}

func (s *Store) ListTenants(status string) ([]PublicTenant, error) {
	status = strings.TrimSpace(status)
	entries, err := s.backend.List(context.Background(), tenantCollection, "")
	if err != nil {
		return nil, err
	}
	var tenants []PublicTenant
	for _, entry := range entries {
		var meta TenantMeta
		if err := json.Unmarshal(entry.Value, &meta); err == nil && ValidSlug(meta.Username) {
			if status == "" || meta.Status == status {
				tenants = append(tenants, meta.Public())
			}
		}
	}
	sort.Slice(tenants, func(i, j int) bool { return tenants[i].UpdatedAt.After(tenants[j].UpdatedAt) })
	return tenants, nil
}

func (s *Store) ApproveTenant(username string) (PublicTenant, error) {
	return s.setTenantStatus(username, TenantApproved)
}

func (s *Store) DenyTenant(username string) (PublicTenant, error) {
	return s.setTenantStatus(username, TenantDenied)
}

func (s *Store) DeleteTenant(username string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	if !ValidSlug(username) {
		return fmt.Errorf("invalid username %q", username)
	}
	ctx := context.Background()
	sites, err := s.ListSites(username)
	if err != nil {
		return err
	}
	for _, site := range sites {
		if err := s.DeleteSite(username, site.Slug); err != nil {
			return err
		}
	}
	sessions, err := s.backend.List(ctx, sessionCollection, "")
	if err != nil {
		return err
	}
	for _, entry := range sessions {
		var session Session
		if err := json.Unmarshal(entry.Value, &session); err == nil && session.Username == username {
			if err := s.backend.Delete(ctx, sessionCollection, entry.Key); err != nil {
				return err
			}
		}
	}
	return s.backend.Delete(ctx, tenantCollection, username)
}

func (s *Store) AuthenticateTenant(username, password string) (PublicTenant, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	meta, err := s.ReadTenant(username)
	if err != nil {
		return PublicTenant{}, fmt.Errorf("invalid username or password")
	}
	if meta.Status != TenantApproved {
		return PublicTenant{}, fmt.Errorf("tenant %q is %s", username, meta.Status)
	}
	if !verifyPassword(meta.PasswordHash, password) {
		return PublicTenant{}, fmt.Errorf("invalid username or password")
	}
	return meta.Public(), nil
}

func (s *Store) CreateSession(username string, ttl time.Duration) (Session, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !ValidSlug(username) {
		return Session{}, fmt.Errorf("invalid username %q", username)
	}
	token, err := randomHex(16)
	if err != nil {
		return Session{}, err
	}
	now := time.Now().UTC()
	session := Session{
		Token:     token,
		Username:  username,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	b, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return Session{}, err
	}
	return session, s.backend.Put(context.Background(), sessionCollection, token, b)
}

func (s *Store) ReadSession(token string) (Session, error) {
	token = strings.TrimSpace(token)
	var session Session
	if token == "" {
		return session, ErrNotFound
	}
	b, err := s.backend.Get(context.Background(), sessionCollection, token)
	if err != nil {
		return session, err
	}
	if err := json.Unmarshal(b, &session); err != nil {
		return session, err
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		_ = s.DeleteSession(token)
		return Session{}, ErrNotFound
	}
	return session, nil
}

func (s *Store) DeleteSession(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return s.backend.Delete(context.Background(), sessionCollection, token)
}

func (s *Store) ReadTenant(username string) (TenantMeta, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	var meta TenantMeta
	if !ValidSlug(username) {
		return meta, fmt.Errorf("invalid username %q", username)
	}
	b, err := s.backend.Get(context.Background(), tenantCollection, username)
	if err != nil {
		return meta, err
	}
	return meta, json.Unmarshal(b, &meta)
}

func (s *Store) CreateSite(tenant, slug, title string) (SiteMeta, error) {
	if err := validateTenant(tenant); err != nil {
		return SiteMeta{}, err
	}
	if !ValidSlug(slug) {
		return SiteMeta{}, fmt.Errorf("invalid slug %q: use lowercase letters, numbers, and dashes", slug)
	}
	now := time.Now().UTC()
	mode := strings.TrimSpace(s.defaultSiteAuthMode)
	if mode == "" {
		mode = SiteAuthOwner
	}
	meta := SiteMeta{Slug: slug, Title: title, Auth: SiteAuthPolicy{Mode: mode}, CreatedAt: now, UpdatedAt: now}
	if existing, err := s.ReadMeta(tenant, slug); err == nil {
		meta.CreatedAt = existing.CreatedAt
		meta.Auth = existing.Auth
	}
	if _, err := s.ReadSiteFile(tenant, slug, "index.html"); errors.Is(err, ErrNotFound) {
		if err := s.WriteSiteFile(tenant, slug, "index.html", []byte(s.defaultIndex)); err != nil {
			return SiteMeta{}, err
		}
	} else if err != nil {
		return SiteMeta{}, err
	}
	return meta, s.writeMeta(tenant, meta)
}

func (s *Store) ListSites(tenant string) ([]SiteMeta, error) {
	if err := validateTenant(tenant); err != nil {
		return nil, err
	}
	entries, err := s.backend.List(context.Background(), siteMetaCollection(tenant), "")
	if err != nil {
		return nil, err
	}
	sites := []SiteMeta{}
	for _, entry := range entries {
		var meta SiteMeta
		if err := json.Unmarshal(entry.Value, &meta); err == nil && ValidSlug(meta.Slug) {
			meta = normalizeSiteMeta(meta)
			sites = append(sites, meta)
		}
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].UpdatedAt.After(sites[j].UpdatedAt) })
	return sites, nil
}

func (s *Store) DeleteSite(tenant, slug string) error {
	if err := validateTenant(tenant); err != nil {
		return err
	}
	if !ValidSlug(slug) {
		return fmt.Errorf("invalid slug %q", slug)
	}
	ctx := context.Background()
	if err := s.backend.Delete(ctx, siteMetaCollection(tenant), slug); err != nil {
		return err
	}
	for _, collection := range []string{siteFilesCollection(tenant, slug), siteDataCollection(tenant, slug), siteUploadsCollection(tenant, slug)} {
		if err := s.backend.DeleteCollection(ctx, collection); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ReadSiteFile(tenant, slug, p string) ([]byte, error) {
	if err := validateTenant(tenant); err != nil {
		return nil, err
	}
	if !ValidSlug(slug) {
		return nil, fmt.Errorf("invalid slug %q", slug)
	}
	clean, err := CleanPath(p)
	if err != nil {
		return nil, err
	}
	return s.backend.Get(context.Background(), siteFilesCollection(tenant, slug), clean)
}

func (s *Store) ListSiteFiles(tenant, slug, prefix string) ([]SiteFileInfo, error) {
	if err := validateTenant(tenant); err != nil {
		return nil, err
	}
	if !ValidSlug(slug) {
		return nil, fmt.Errorf("invalid slug %q", slug)
	}
	cleanPrefix, err := CleanPrefix(prefix)
	if err != nil {
		return nil, err
	}
	entries, err := s.backend.List(context.Background(), siteFilesCollection(tenant, slug), cleanPrefix)
	if err != nil {
		return nil, err
	}
	files := make([]SiteFileInfo, 0, len(entries))
	for _, entry := range entries {
		files = append(files, SiteFileInfo{Path: entry.Key, Size: len(entry.Value)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func (s *Store) WriteSiteFile(tenant, slug, p string, b []byte) error {
	if err := validateTenant(tenant); err != nil {
		return err
	}
	if !ValidSlug(slug) {
		return fmt.Errorf("invalid slug %q", slug)
	}
	clean, err := CleanPath(p)
	if err != nil {
		return err
	}
	if err := s.backend.Put(context.Background(), siteFilesCollection(tenant, slug), clean, b); err != nil {
		return err
	}
	if meta, err := s.ReadMeta(tenant, slug); err == nil {
		meta.UpdatedAt = time.Now().UTC()
		_ = s.writeMeta(tenant, meta)
	}
	return nil
}

func (s *Store) DeleteSiteFile(tenant, slug, p string) error {
	if err := validateTenant(tenant); err != nil {
		return err
	}
	if !ValidSlug(slug) {
		return fmt.Errorf("invalid slug %q", slug)
	}
	clean, err := CleanPath(p)
	if err != nil {
		return err
	}
	if err := s.backend.Delete(context.Background(), siteFilesCollection(tenant, slug), clean); err != nil {
		return err
	}
	if meta, err := s.ReadMeta(tenant, slug); err == nil {
		meta.UpdatedAt = time.Now().UTC()
		_ = s.writeMeta(tenant, meta)
	}
	return nil
}

func (s *Store) ReadMeta(tenant, slug string) (SiteMeta, error) {
	var meta SiteMeta
	if err := validateTenant(tenant); err != nil {
		return meta, err
	}
	if !ValidSlug(slug) {
		return meta, fmt.Errorf("invalid slug %q", slug)
	}
	b, err := s.backend.Get(context.Background(), siteMetaCollection(tenant), slug)
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		return meta, err
	}
	return normalizeSiteMeta(meta), nil
}

func (s *Store) UpdateSiteAuth(tenant, slug string, policy SiteAuthPolicy) (SiteMeta, error) {
	if err := validateTenant(tenant); err != nil {
		return SiteMeta{}, err
	}
	if !ValidSlug(slug) {
		return SiteMeta{}, fmt.Errorf("invalid slug %q", slug)
	}
	meta, err := s.ReadMeta(tenant, slug)
	if err != nil {
		return SiteMeta{}, err
	}
	normalized, err := normalizeSiteAuthPolicy(policy)
	if err != nil {
		return SiteMeta{}, err
	}
	meta.Auth = normalized
	meta.UpdatedAt = time.Now().UTC()
	if err := s.writeMeta(tenant, meta); err != nil {
		return SiteMeta{}, err
	}
	return meta, nil
}

func (s *Store) ReadData(tenant, slug string) (map[string]any, error) {
	if err := validateTenant(tenant); err != nil {
		return nil, err
	}
	if !ValidSlug(slug) {
		return nil, fmt.Errorf("invalid slug %q", slug)
	}
	entries, err := s.backend.List(context.Background(), siteDataCollection(tenant, slug), "")
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, entry := range entries {
		var value any
		if err := json.Unmarshal(entry.Value, &value); err != nil {
			return nil, err
		}
		out[entry.Key] = value
	}
	return out, nil
}

func (s *Store) WriteData(tenant, slug string, data map[string]any) error {
	if err := validateTenant(tenant); err != nil {
		return err
	}
	if !ValidSlug(slug) {
		return fmt.Errorf("invalid slug %q", slug)
	}
	ctx := context.Background()
	collection := siteDataCollection(tenant, slug)
	existing, err := s.backend.List(ctx, collection, "")
	if err != nil {
		return err
	}
	next := map[string]bool{}
	for key, value := range data {
		key = strings.Trim(key, "/")
		if key == "" {
			continue
		}
		b, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if err := s.backend.Put(ctx, collection, key, b); err != nil {
			return err
		}
		next[key] = true
	}
	for _, entry := range existing {
		if !next[entry.Key] {
			if err := s.backend.Delete(ctx, collection, entry.Key); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) SaveUpload(tenant, slug, originalName string, r io.Reader) (Upload, error) {
	if err := validateTenant(tenant); err != nil {
		return Upload{}, err
	}
	if !ValidSlug(slug) {
		return Upload{}, fmt.Errorf("invalid slug %q", slug)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return Upload{}, err
	}
	id, err := randomHex(8)
	if err != nil {
		return Upload{}, err
	}
	name := id + filepath.Ext(originalName)
	if err := s.backend.Put(context.Background(), siteUploadsCollection(tenant, slug), name, b); err != nil {
		return Upload{}, err
	}
	return Upload{URL: "/uploads/" + tenant + "/" + slug + "/" + name, Name: originalName}, nil
}

func (s *Store) ReadUpload(tenant, slug, name string) ([]byte, error) {
	if err := validateTenant(tenant); err != nil {
		return nil, err
	}
	if !ValidSlug(slug) {
		return nil, fmt.Errorf("invalid slug %q", slug)
	}
	name, err := CleanPath(name)
	if err != nil {
		return nil, err
	}
	return s.backend.Get(context.Background(), siteUploadsCollection(tenant, slug), name)
}

func (s *Store) ListUploads(tenant, slug string) ([]UploadInfo, error) {
	if err := validateTenant(tenant); err != nil {
		return nil, err
	}
	if !ValidSlug(slug) {
		return nil, fmt.Errorf("invalid slug %q", slug)
	}
	entries, err := s.backend.List(context.Background(), siteUploadsCollection(tenant, slug), "")
	if err != nil {
		return nil, err
	}
	uploads := make([]UploadInfo, 0, len(entries))
	for _, entry := range entries {
		uploads = append(uploads, UploadInfo{
			Name: entry.Key,
			URL:  "/uploads/" + tenant + "/" + slug + "/" + entry.Key,
			Size: len(entry.Value),
		})
	}
	sort.Slice(uploads, func(i, j int) bool { return uploads[i].Name < uploads[j].Name })
	return uploads, nil
}

func (s *Store) DeleteUpload(tenant, slug, name string) error {
	if err := validateTenant(tenant); err != nil {
		return err
	}
	if !ValidSlug(slug) {
		return fmt.Errorf("invalid slug %q", slug)
	}
	name, err := CleanPath(name)
	if err != nil {
		return err
	}
	if err := s.backend.Delete(context.Background(), siteUploadsCollection(tenant, slug), name); err != nil {
		return err
	}
	if meta, err := s.ReadMeta(tenant, slug); err == nil {
		meta.UpdatedAt = time.Now().UTC()
		_ = s.writeMeta(tenant, meta)
	}
	return nil
}

func (s *Store) setTenantStatus(username, status string) (PublicTenant, error) {
	meta, err := s.ReadTenant(username)
	if err != nil {
		return PublicTenant{}, err
	}
	meta.Status = status
	meta.UpdatedAt = time.Now().UTC()
	if err := s.writeTenant(meta); err != nil {
		return PublicTenant{}, err
	}
	return meta.Public(), nil
}

func (s *Store) writeTenant(meta TenantMeta) error {
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return s.backend.Put(context.Background(), tenantCollection, meta.Username, b)
}

func (s *Store) writeMeta(tenant string, meta SiteMeta) error {
	meta = normalizeSiteMeta(meta)
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return s.backend.Put(context.Background(), siteMetaCollection(tenant), meta.Slug, b)
}

func (t TenantMeta) Public() PublicTenant {
	return PublicTenant{
		Username:  t.Username,
		Status:    t.Status,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}
