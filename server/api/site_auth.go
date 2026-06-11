package api

import (
	"fmt"
	"sort"
	"strings"
)

func (p SiteAuthPolicy) Allows(ownerTenant, username string, authenticated bool) bool {
	switch p.Mode {
	case SiteAuthNone:
		return true
	case SiteAuthOwner:
		return authenticated && username == ownerTenant
	case SiteAuthTenants:
		if !authenticated {
			return false
		}
		if len(p.Tenants) == 0 {
			return true
		}
		for _, tenant := range p.Tenants {
			if tenant == username {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func defaultSiteAuthPolicy() SiteAuthPolicy {
	return SiteAuthPolicy{Mode: SiteAuthOwner}
}

func normalizeSiteMeta(meta SiteMeta) SiteMeta {
	policy, err := normalizeSiteAuthPolicy(meta.Auth)
	if err != nil {
		policy = defaultSiteAuthPolicy()
	}
	meta.Auth = policy
	if meta.Auth.Mode != SiteAuthOwner {
		meta.AgentMessages = false
	}
	return meta
}

func normalizeSiteAuthPolicy(policy SiteAuthPolicy) (SiteAuthPolicy, error) {
	mode := strings.ToLower(strings.TrimSpace(policy.Mode))
	if mode == "" {
		return defaultSiteAuthPolicy(), nil
	}
	switch mode {
	case SiteAuthNone:
		return SiteAuthPolicy{Mode: SiteAuthNone}, nil
	case SiteAuthOwner:
		return SiteAuthPolicy{Mode: SiteAuthOwner}, nil
	case SiteAuthTenants:
		tenants, err := normalizeSiteAuthTenants(policy.Tenants)
		if err != nil {
			return SiteAuthPolicy{}, err
		}
		return SiteAuthPolicy{Mode: SiteAuthTenants, Tenants: tenants}, nil
	default:
		return SiteAuthPolicy{}, fmt.Errorf("invalid auth mode %q", policy.Mode)
	}
}

func normalizeSiteAuthTenants(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	tenants := []string{}
	for _, tenant := range raw {
		tenant = strings.ToLower(strings.TrimSpace(tenant))
		if tenant == "" {
			continue
		}
		if !ValidSlug(tenant) {
			return nil, fmt.Errorf("invalid tenant %q", tenant)
		}
		if !seen[tenant] {
			seen[tenant] = true
			tenants = append(tenants, tenant)
		}
	}
	sort.Strings(tenants)
	if len(tenants) == 0 {
		return nil, nil
	}
	return tenants, nil
}
