package api

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/nerdneilsfield/aumbry/internal/auth"
)

// clientIPMiddleware resolves the client IP and whether the connection arrived via HTTPS.
//
// When trusted is empty, forwarding headers are ignored and the TCP peer is always used.
// When trusted is non-empty, X-Forwarded-For and X-Forwarded-Proto are consulted only if
// the direct TCP peer (r.RemoteAddr) falls within trusted; otherwise they are ignored.
func clientIPMiddleware(trusted []netip.Prefix) func(http.Handler) http.Handler {
	if len(trusted) == 0 {
		return middleware.ClientIPFromRemoteAddr
	}
	prefixes := make([]string, len(trusted))
	for i, p := range trusted {
		prefixes[i] = p.String()
	}
	xff := middleware.ClientIPFromXFF(prefixes...)

	return func(next http.Handler) http.Handler {
		remoteFallback := middleware.ClientIPFromRemoteAddr(next)
		trustedChain := xff(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !middleware.GetClientIPAddr(r.Context()).IsValid() {
				remoteFallback.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		}))

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer, ok := parseRemoteAddr(r.RemoteAddr)
			if !ok || !inAnyPrefix(peer, trusted) {
				remoteFallback.ServeHTTP(w, r)
				return
			}
			if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
				r = r.WithContext(auth.WithSecureRequest(r.Context(), true))
			}
			trustedChain.ServeHTTP(w, r)
		})
	}
}

func parseRemoteAddr(raw string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(raw)
	if err != nil {
		host = raw
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap().WithZone(""), true
}

func inAnyPrefix(ip netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
