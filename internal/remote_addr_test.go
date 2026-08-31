package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteAddr(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    http.Header
		want       string
	}{
		{
			name:       "no headers strips the port",
			remoteAddr: "203.0.113.7:52814",
			want:       "203.0.113.7",
		},
		{
			name:       "no headers and no port",
			remoteAddr: "203.0.113.7",
			want:       "203.0.113.7",
		},
		{
			name:       "no headers keeps an unparseable remote addr",
			remoteAddr: "@",
			want:       "@",
		},
		{
			name:       "single x-forwarded-for",
			remoteAddr: "10.0.0.1:80",
			headers:    http.Header{"X-Forwarded-For": {"198.51.100.4"}},
			want:       "198.51.100.4",
		},
		{
			name:       "x-forwarded-for with a port",
			remoteAddr: "10.0.0.1:80",
			headers:    http.Header{"X-Forwarded-For": {"198.51.100.4:8080"}},
			want:       "198.51.100.4",
		},
		{
			name:       "chained x-forwarded-for skips private hops",
			remoteAddr: "127.0.0.1:80",
			headers:    http.Header{"X-Forwarded-For": {"203.0.113.9, 10.0.0.1, 192.168.1.5"}},
			want:       "203.0.113.9",
		},
		{
			name:       "spoofed prefix loses to the rightmost public hop",
			remoteAddr: "127.0.0.1:80",
			headers:    http.Header{"X-Forwarded-For": {"1.2.3.4, 203.0.113.9, 172.16.4.4"}},
			want:       "203.0.113.9",
		},
		{
			name:       "x-forwarded-for split over two headers",
			remoteAddr: "127.0.0.1:80",
			headers:    http.Header{"X-Forwarded-For": {"203.0.113.9", "10.0.0.1"}},
			want:       "203.0.113.9",
		},
		{
			name:       "invalid tokens are skipped",
			remoteAddr: "10.0.0.1:80",
			headers:    http.Header{"X-Forwarded-For": {"garbage, 198.51.100.4, unknown, 10.0.0.1:port"}},
			want:       "198.51.100.4",
		},
		{
			name:       "only invalid tokens fall back to the remote addr",
			remoteAddr: "203.0.113.7:443",
			headers:    http.Header{"X-Forwarded-For": {"garbage, unknown"}},
			want:       "203.0.113.7",
		},
		{
			name:       "ipv6 with brackets",
			remoteAddr: "[::1]:80",
			headers:    http.Header{"X-Forwarded-For": {"[2001:db8::2]"}},
			want:       "2001:db8::2",
		},
		{
			name:       "ipv6 skips loopback and unique local hops",
			remoteAddr: "[::1]:80",
			headers:    http.Header{"X-Forwarded-For": {"2001:db8::2, fd00::5, ::1"}},
			want:       "2001:db8::2",
		},
		{
			name:       "ipv4 mapped ipv6 counts as its ipv4 range",
			remoteAddr: "[::1]:80",
			headers:    http.Header{"X-Forwarded-For": {"203.0.113.9, ::ffff:10.0.0.1"}},
			want:       "203.0.113.9",
		},
		{
			name:       "all private chain uses the leftmost hop",
			remoteAddr: "127.0.0.1:80",
			headers:    http.Header{"X-Forwarded-For": {"10.1.2.3, 192.168.0.9, 172.16.0.1"}},
			want:       "10.1.2.3",
		},
		{
			name:       "all private ipv6 chain uses the leftmost hop",
			remoteAddr: "[::1]:80",
			headers:    http.Header{"X-Forwarded-For": {"fd12::1, fe80::9"}},
			want:       "fd12::1",
		},
		{
			name:       "forwarded header",
			remoteAddr: "10.0.0.1:80",
			headers:    http.Header{"Forwarded": {"for=198.51.100.17;proto=https;by=203.0.113.43"}},
			want:       "198.51.100.17",
		},
		{
			name:       "forwarded header with quoted ipv6 and port",
			remoteAddr: "10.0.0.1:80",
			headers:    http.Header{"Forwarded": {`for=192.168.0.2, for="[2001:db8:cafe::17]:4711"`}},
			want:       "2001:db8:cafe::17",
		},
		{
			name:       "forwarded key is case insensitive",
			remoteAddr: "10.0.0.1:80",
			headers:    http.Header{"Forwarded": {`For="203.0.113.60:4711"`}},
			want:       "203.0.113.60",
		},
		{
			name:       "forwarded wins over x-forwarded-for",
			remoteAddr: "10.0.0.1:80",
			headers: http.Header{
				"Forwarded":       {"for=198.51.100.17"},
				"X-Forwarded-For": {"203.0.113.9"},
			},
			want: "198.51.100.17",
		},
		{
			name:       "forwarded with only obfuscated hops falls back to x-forwarded-for",
			remoteAddr: "10.0.0.1:80",
			headers: http.Header{
				"Forwarded":       {"for=unknown, for=_hidden"},
				"X-Forwarded-For": {"203.0.113.9"},
			},
			want: "203.0.113.9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			for key, values := range tt.headers {
				for _, value := range values {
					r.Header.Add(key, value)
				}
			}
			if got := RemoteAddr(r); got != tt.want {
				t.Fatalf("RemoteAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRemoteAddrNilRequest(t *testing.T) {
	if got := RemoteAddr(nil); got != "" {
		t.Fatalf("RemoteAddr(nil) = %q, want empty", got)
	}
}
