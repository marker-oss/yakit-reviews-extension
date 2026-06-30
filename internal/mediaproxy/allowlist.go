// internal/mediaproxy/allowlist.go
package mediaproxy

import (
	"net/url"
	"strings"
)

// HostAllowed reports whether rawURL is an https URL whose host equals or is a
// subdomain of one of the allowed suffixes. Guards the proxy against
// open-proxy/SSRF abuse.
func HostAllowed(rawURL string, suffixes []string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, s := range suffixes {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}
