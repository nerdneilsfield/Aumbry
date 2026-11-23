package schema

import (
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

var extraNonPublicPrefixes = mustParsePrefixes([]string{
	"0.0.0.0/8",
	"100.64.0.0/10",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"240.0.0.0/4",
	"64:ff9b::/96",
	"64:ff9b:1::/48",
	"100::/64",
	"2001::/23",
	"2001:db8::/32",
	"2002::/16",
	"fec0::/10",
})

func mustParsePrefixes(cidrs []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, s := range cidrs {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

// IsPublicAddr reports whether addr (after unmapping IPv4-mapped IPv6 addresses) is a
// publicly routable unicast IP address permitted for outbound remote image fetching.
func IsPublicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() {
		return false
	}
	if addr.IsPrivate() || addr.IsLoopback() || addr.IsUnspecified() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() || addr.IsInterfaceLocalMulticast() {
		return false
	}
	for _, p := range extraNonPublicPrefixes {
		if p.Contains(addr) {
			return false
		}
	}
	return true
}

func isPureASCII(s string) bool {
	for i := range len(s) {
		if s[i] > 0x7f {
			return false
		}
	}
	return true
}

func isValidRelaxedASCIIHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := range len(label) {
			c := label[i]
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
				return false
			}
		}
	}
	return true
}

// IsRemoteURLCandidate reports whether raw starts with or parses as a remote URL scheme.
func IsRemoteURLCandidate(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") ||
		strings.Contains(lower, "://") {
		return true
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme != ""
}

// ValidateRemoteImageURL verifies offline SSRF syntax rules on a remote image URL
// without performing any network or DNS requests.
func ValidateRemoteImageURL(raw string) (code, msg, hint string, ok bool) {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return "REMOTE_ASSET_BLOCKED", fmt.Sprintf("remote figure url %q contains surrounding whitespace or is empty", raw), "use a clean https:// or http:// URL without whitespace", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "REMOTE_ASSET_BLOCKED", fmt.Sprintf("invalid remote figure url %q: %v", raw, err), "use a valid https:// or http:// URL", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "REMOTE_ASSET_BLOCKED", fmt.Sprintf("remote figure url scheme %q is blocked; only http and https are allowed", u.Scheme), "use an https:// or http:// image URL", false
	}
	if u.User != nil {
		return "REMOTE_ASSET_BLOCKED", "remote figure url must not contain userinfo credentials", "remove username/password from the image URL", false
	}
	rawHost := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if rawHost == "" {
		return "REMOTE_ASSET_BLOCKED", "remote figure url must include a host", "use a valid https:// or http:// URL with a public host", false
	}
	if ip, ipErr := netip.ParseAddr(rawHost); ipErr == nil {
		if !IsPublicAddr(ip) {
			return "REMOTE_ASSET_BLOCKED", fmt.Sprintf("remote figure IP %s is non-public and blocked", ip.Unmap()), "remote images must target public IP addresses", false
		}
		return "", "", "", true
	}
	host := rawHost
	if asciiHost, idnaErr := idna.Lookup.ToASCII(rawHost); idnaErr == nil {
		host = strings.TrimSuffix(strings.ToLower(asciiHost), ".")
	} else if !isPureASCII(rawHost) || !isValidRelaxedASCIIHost(rawHost) {
		return "REMOTE_ASSET_BLOCKED", fmt.Sprintf("remote figure host %q is invalid: %v", u.Hostname(), idnaErr), "use a standard ASCII domain name or public IP", false
	}
	if host == "" {
		return "REMOTE_ASSET_BLOCKED", "remote figure url must include a host", "use a valid https:// or http:// URL with a public host", false
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "localhost.localdomain" {
		return "REMOTE_ASSET_BLOCKED", fmt.Sprintf("remote figure host %q is blocked", u.Hostname()), "remote images must target public hosts", false
	}
	if ip, ipErr := netip.ParseAddr(host); ipErr == nil {
		if !IsPublicAddr(ip) {
			return "REMOTE_ASSET_BLOCKED", fmt.Sprintf("remote figure IP %s is non-public and blocked", ip.Unmap()), "remote images must target public IP addresses", false
		}
		return "", "", "", true
	}
	if looksLikeNonCanonicalNumericIP(host) {
		return "REMOTE_ASSET_BLOCKED", fmt.Sprintf("non-canonical numeric host %q is blocked", u.Hostname()), "use a standard hostname or public IP address", false
	}
	if !isValidRelaxedASCIIHost(host) {
		return "REMOTE_ASSET_BLOCKED", fmt.Sprintf("remote figure host %q is invalid", u.Hostname()), "use a standard ASCII domain name or public IP", false
	}
	return "", "", "", true
}

func looksLikeNonCanonicalNumericIP(host string) bool {
	if _, err := strconv.ParseUint(host, 0, 64); err == nil {
		return true
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 || len(parts) > 4 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		if _, err := strconv.ParseUint(p, 0, 64); err != nil {
			return false
		}
	}
	return true
}
