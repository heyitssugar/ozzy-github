package extractor

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	unicodeEscapeRe = regexp.MustCompile(`(?i)\\u00[0-9a-f]{2}`)
	hexPrefixRe     = regexp.MustCompile(`^(?:252f|2f)`)
	nonDomainCharRe  = regexp.MustCompile(`[^a-z0-9._-]`)
	protocolRe       = regexp.MustCompile(`^https?://`)
	wildcardPrefixRe = regexp.MustCompile(`^\*\.`)
)

// CleanSubdomain normalizes a raw subdomain match.
// Pipeline: lowercase → strip protocol → strip path → URL decode → strip unicode escapes → trim → validate.
func CleanSubdomain(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))

	// Strip protocol prefix (https://, http://)
	s = protocolRe.ReplaceAllString(s, "")

	// Strip URL path suffix — only after a domain (contains at least one dot before the slash)
	if idx := strings.Index(s, "/"); idx > 0 && strings.Contains(s[:idx], ".") {
		s = s[:idx]
	}

	// Strip wildcard prefix (*.)
	s = wildcardPrefixRe.ReplaceAllString(s, "")

	// URL decode (handles %2F, %252F, etc.)
	if decoded, err := url.QueryUnescape(s); err == nil {
		s = decoded
	}
	// Double decode for %252F → %2F → /
	if decoded, err := url.QueryUnescape(s); err == nil {
		s = decoded
	}

	// Strip unicode escape sequences like \u002f
	s = unicodeEscapeRe.ReplaceAllString(s, "")

	// Strip hex prefixes that remain (2f, 252f)
	s = hexPrefixRe.ReplaceAllString(s, "")

	// Trim leading/trailing dots and slashes
	s = strings.Trim(s, "./\\")

	// Remove any remaining non-domain characters
	s = nonDomainCharRe.ReplaceAllString(s, "")

	// Trim dots again after cleaning
	s = strings.Trim(s, ".")

	// Validate label lengths
	if !isValidDomain(s) {
		return ""
	}

	return s
}

// isValidDomain checks basic DNS domain validity:
// - total length <= 253
// - each label 1-63 chars
// - at least one dot (has a TLD)
func isValidDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}

	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}

	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
	}

	return true
}
