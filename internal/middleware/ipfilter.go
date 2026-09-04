package middleware

import (
	"net"
	"net/http"
	"strings"
)

// common for both
type IPFilterConfig struct {
	Whitelist      []string
	Blacklist      []string
	TrustedProxies []string
}

func DefaultIPFilterConfig() IPFilterConfig {
	return IPFilterConfig{
		Whitelist:      []string{},
		Blacklist:      []string{},
		TrustedProxies: []string{"127.0.0.1", "::1"},
	}
}

func IPFilterMiddleware(config IPFilterConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := getClientIP(r)

			if len(config.Blacklist) > 0 && isIPInList(clientIP, config.Blacklist) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			if len(config.Whitelist) > 0 && !isIPInList(clientIP, config.Whitelist) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if ip != "" {
				return ip
			}
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func isIPInList(ipStr string, ipList []string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, item := range ipList {
		if strings.Contains(item, "/") {
			_, ipNet, err := net.ParseCIDR(item)
			if err != nil {
				continue
			}
			if ipNet.Contains(ip) {
				return true
			}
		} else {
			if item == ipStr {
				return true
			}
		}
	}

	return false
}
