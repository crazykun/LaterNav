package handlers

import (
	"net/url"
	"strings"
)

// oidcAdminURL validates an admin-entered endpoint URL.
// Issuers must be HTTPS; other endpoints also allow localhost HTTP for development.
func oidcAdminURL(raw string, issuer bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || u.RawPath != "" {
		return false
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && !issuer && u.Hostname() == "localhost") {
		return false
	}
	if issuer && strings.HasSuffix(u.Path, "/") {
		return false
	}
	return true
}
