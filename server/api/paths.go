package api

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func ValidSlug(slug string) bool {
	return slugRe.MatchString(slug)
}

func CleanPath(p string) (string, error) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		p = "index.html"
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", fmt.Errorf("invalid path")
		}
	}
	return strings.TrimPrefix(path.Clean("/"+p), "/"), nil
}

func CleanPrefix(p string) (string, error) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", nil
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", fmt.Errorf("invalid path")
		}
	}
	clean := strings.TrimPrefix(path.Clean("/"+p), "/")
	if clean == "." {
		return "", nil
	}
	if strings.HasSuffix(p, "/") && clean != "" {
		clean += "/"
	}
	return clean, nil
}

const (
	tenantCollection  = "flink/tenants"
	sessionCollection = "flink/sessions"
)

func siteMetaCollection(tenant string) string {
	return "tenants/" + tenant + "/site-meta"
}

func siteFilesCollection(tenant, slug string) string {
	return "tenants/" + tenant + "/sites/" + slug + "/files"
}

func siteDataCollection(tenant, slug string) string {
	return "tenants/" + tenant + "/sites/" + slug + "/data"
}

func siteUploadsCollection(tenant, slug string) string {
	return "tenants/" + tenant + "/sites/" + slug + "/uploads"
}

func sitePublishesCollection(tenant, slug string) string {
	return "tenants/" + tenant + "/sites/" + slug + "/publishes"
}

func sitePublishFilesCollection(tenant, slug, version string) string {
	return "tenants/" + tenant + "/sites/" + slug + "/publish-files/" + version
}

func validateTenant(tenant string) error {
	if !ValidSlug(tenant) {
		return fmt.Errorf("invalid tenant %q", tenant)
	}
	return nil
}
