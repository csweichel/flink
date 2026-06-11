package cmd

import "time"

type siteMeta struct {
	Slug      string         `json:"slug"`
	Title     string         `json:"title"`
	Auth      siteAuthPolicy `json:"auth"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type siteAuthPolicy struct {
	Mode    string   `json:"mode"`
	Tenants []string `json:"tenants,omitempty"`
}

type siteFileInfo struct {
	Path string `json:"path"`
	Size int    `json:"size"`
}
