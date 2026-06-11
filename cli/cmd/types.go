package cmd

import "time"

type siteMeta struct {
	Slug              string         `json:"slug"`
	Title             string         `json:"title"`
	Auth              siteAuthPolicy `json:"auth"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
	CreatedBy         string         `json:"createdBy,omitempty"`
	UpdatedBy         string         `json:"updatedBy,omitempty"`
	LastPublishedBy   string         `json:"lastPublishedBy,omitempty"`
	LastPublishedAt   time.Time      `json:"lastPublishedAt,omitempty"`
	LastPublishedFrom string         `json:"lastPublishedFrom,omitempty"`
	LastGitCommit     string         `json:"lastGitCommit,omitempty"`
	FileCount         int            `json:"fileCount,omitempty"`
	TotalBytes        int            `json:"totalBytes,omitempty"`
	Capabilities      []string       `json:"capabilities,omitempty"`
}

type siteAuthPolicy struct {
	Mode    string   `json:"mode"`
	Tenants []string `json:"tenants,omitempty"`
}

type siteFileInfo struct {
	Path string `json:"path"`
	Size int    `json:"size"`
}

type siteDetails struct {
	Site    siteMeta       `json:"site"`
	Files   []siteFileInfo `json:"files"`
	Data    map[string]any `json:"data"`
	Uploads []uploadInfo   `json:"uploads"`
}

type uploadInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int    `json:"size"`
}

type publishFileInfo struct {
	Path string `json:"path"`
	Size int    `json:"size"`
	Hash string `json:"hash"`
}

type publishRecord struct {
	ID           string            `json:"id,omitempty"`
	CreatedAt    time.Time         `json:"createdAt,omitempty"`
	Tenant       string            `json:"tenant,omitempty"`
	Source       string            `json:"source,omitempty"`
	GitCommit    string            `json:"gitCommit,omitempty"`
	FileCount    int               `json:"fileCount"`
	TotalBytes   int               `json:"totalBytes"`
	Files        []publishFileInfo `json:"files"`
	Auth         siteAuthPolicy    `json:"auth"`
	RollbackOf   string            `json:"rollbackOf,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
}
