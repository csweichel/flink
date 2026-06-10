package cmd

import "time"

type siteMeta struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type siteFileInfo struct {
	Path string `json:"path"`
	Size int    `json:"size"`
}
