// Package siteurl normalizes the public origin used by storefront SEO outputs.
package siteurl

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// Normalize accepts legacy bare domains, but never credentials, paths or queries.
// Public origins use HTTPS; HTTP remains available for local development only.
func Normalize(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("site URL is empty")
	}
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	} else if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("site URL must contain a valid host")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("site URL must use HTTPS")
	}
	if u.User != nil || strings.ContainsAny(raw, "?#\\") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("site URL must be an origin without credentials, path, query or fragment")
	}
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	local := host == "localhost" || host == "127.0.0.1" || host == "::1"
	if !local {
		u.Scheme = "https"
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 0 || n > 65535 {
			return "", fmt.Errorf("site URL has an invalid port")
		}
	}
	if u.Scheme == "https" && port == "443" {
		port = ""
	}
	u.Host = host
	if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	}
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	}
	u.Path = ""
	u.RawPath = ""
	return u.String(), nil
}
