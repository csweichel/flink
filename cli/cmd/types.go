package cmd

import "time"

type siteMeta struct {
	Slug              string         `json:"slug"`
	Title             string         `json:"title"`
	Auth              siteAuthPolicy `json:"auth"`
	AgentMessages     bool           `json:"agentMessages,omitempty"`
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

type siteFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type siteDetails struct {
	Site    siteMeta       `json:"site"`
	Files   []siteFileInfo `json:"files"`
	Data    map[string]any `json:"data"`
	Uploads []uploadInfo   `json:"uploads"`
}

type uploadResult struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type uploadInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int    `json:"size"`
}

type aiRequest struct {
	Prompt          string `json:"prompt"`
	Instructions    string `json:"instructions,omitempty"`
	Model           string `json:"model,omitempty"`
	MaxOutputTokens int    `json:"maxOutputTokens,omitempty"`
}

type aiResponse struct {
	Text       string `json:"text"`
	Model      string `json:"model,omitempty"`
	Configured bool   `json:"configured"`
}

type agentStatus struct {
	Enabled   bool `json:"enabled"`
	Listening bool `json:"listening"`
	Pending   int  `json:"pending"`
}

type agentMessage struct {
	ID         string           `json:"id"`
	Tenant     string           `json:"tenant"`
	Site       string           `json:"site"`
	Text       string           `json:"text"`
	Sender     string           `json:"sender,omitempty"`
	Screenshot *agentScreenshot `json:"screenshot,omitempty"`
	CreatedAt  time.Time        `json:"createdAt"`
}

type agentScreenshot struct {
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	DataURL string `json:"dataUrl,omitempty"`
}

type agentResponse struct {
	ID        string    `json:"id"`
	Tenant    string    `json:"tenant"`
	Site      string    `json:"site"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
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
