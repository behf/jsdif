package main

import (
	"strings"
)

// normalizeURL removes trailing slashes from URLs to ensure consistent handling
// This prevents duplicate entries for URLs that differ only by a trailing slash
func normalizeURL(url string) string {
	// Remove trailing slash if present, but preserve the root URL with just a domain
	if len(url) > 0 && strings.HasSuffix(url, "/") && !strings.HasSuffix(url, "://") {
		// Check if there's a path component (not just http:// or https://)
		if strings.Count(url, "/") > 2 {
			return strings.TrimSuffix(url, "/")
		}
	}
	return url
}