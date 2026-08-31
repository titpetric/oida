package internal

import (
	"net/http"
	"net/netip"
	"strings"
)

// trustedNetworks lists the address ranges a forwarding hop can occupy without
// being the client: RFC 1918 private space, loopback and link-local, plus their
// IPv6 counterparts. Addresses in these ranges are infrastructure, not callers.
var trustedNetworks = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
}

// remoteAddr resolves the client IP of a request. It walks the forwarding
// chain from the Forwarded (RFC 7239) or X-Forwarded-For headers right to left
// and returns the first address outside the trusted LAN ranges: the hops a
// proxy appended are trustworthy, anything to their left is client input. When
// every hop is trusted, all-LAN traffic, the leftmost valid address is the
// client. Without a usable header the request's own RemoteAddr decides.
func RemoteAddr(r *http.Request) string {
	if r == nil {
		return ""
	}
	chain := forwardedChain(r)
	for i := len(chain) - 1; i >= 0; i-- {
		if !trustedAddr(chain[i]) {
			return chain[i].String()
		}
	}
	if len(chain) > 0 {
		return chain[0].String()
	}
	if addr, ok := parseForwardedAddr(r.RemoteAddr); ok {
		return addr.String()
	}
	return r.RemoteAddr
}

// forwardedChain collects the forwarding chain from the request headers. The
// standard Forwarded header wins when it names any address; X-Forwarded-For is
// the fallback. Tokens that hold no address, RFC 7239 allows "unknown" and
// obfuscated identifiers, are skipped.
func forwardedChain(r *http.Request) []netip.Addr {
	var chain []netip.Addr
	for _, header := range r.Header.Values("Forwarded") {
		for _, element := range strings.Split(header, ",") {
			for _, pair := range strings.Split(element, ";") {
				key, value, ok := strings.Cut(pair, "=")
				if !ok || !strings.EqualFold(strings.TrimSpace(key), "for") {
					continue
				}
				if addr, ok := parseForwardedAddr(value); ok {
					chain = append(chain, addr)
				}
			}
		}
	}
	if len(chain) > 0 {
		return chain
	}
	for _, header := range r.Header.Values("X-Forwarded-For") {
		for _, token := range strings.Split(header, ",") {
			if addr, ok := parseForwardedAddr(token); ok {
				chain = append(chain, addr)
			}
		}
	}
	return chain
}

// parseForwardedAddr parses one hop of a forwarding chain. It tolerates
// surrounding whitespace and quotes, an attached port, IPv6 brackets and
// zones, and reports anything else as not ok.
func parseForwardedAddr(token string) (netip.Addr, bool) {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, `"`)
	if token == "" {
		return netip.Addr{}, false
	}
	if addrPort, err := netip.ParseAddrPort(token); err == nil {
		return addrPort.Addr().Unmap().WithZone(""), true
	}
	token = strings.TrimPrefix(token, "[")
	token = strings.TrimSuffix(token, "]")
	if addr, err := netip.ParseAddr(token); err == nil {
		return addr.Unmap().WithZone(""), true
	}
	return netip.Addr{}, false
}

// trustedAddr reports whether addr sits in one of the trusted LAN ranges.
func trustedAddr(addr netip.Addr) bool {
	for _, network := range trustedNetworks {
		if network.Contains(addr) {
			return true
		}
	}
	return false
}
