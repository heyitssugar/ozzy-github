package extractor

import (
	"regexp"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// Subdomain represents a discovered subdomain with its source.
type Subdomain struct {
	Value  string // The cleaned subdomain (e.g., "api.example.com")
	Source string // Where it was found (e.g., GitHub URL)
}

// Extractor finds subdomains in text content.
type Extractor interface {
	Extract(content string, source string) []Subdomain
}

// RegexpExtractor uses multiple compiled regular expressions to find subdomains.
type RegexpExtractor struct {
	patterns []*regexp.Regexp
	domain   string
	extend   bool
}

// NewRegexpExtractor creates an extractor for the given domain.
// The domain can be an apex domain (example.com) or include a subdomain (api.example.com).
// If extend is true, also matches variants like <prefix>example.<tld>.
func NewRegexpExtractor(domain string, extend bool) (*RegexpExtractor, error) {
	parsedDomain, parsedTLD := splitDomainTLD(domain)
	_ = parsedTLD

	var patterns []string

	if extend {
		// Extended mode: match things like testexample.com, example-test.co.uk
		patterns = append(patterns,
			`(?i)(?:[0-9a-z_](?:[0-9a-z_\-]{0,61}[0-9a-z_])?\.)*`+
				`(?:[0-9a-z_\-]*)?`+regexp.QuoteMeta(parsedDomain)+`(?:[0-9a-z_\-]*)?`+
				`\.(?:[a-z]{2,63})(?:\.[a-z]{2,63})?`,
		)
	} else {
		// Pattern 1: Standard subdomain pattern — *.domain.tld
		escapedFull := regexp.QuoteMeta(domain)
		patterns = append(patterns,
			`(?i)(?:[0-9a-z_](?:[0-9a-z_\-]{0,61}[0-9a-z_])?\.)*`+escapedFull,
		)
	}

	// Pattern 2: URL pattern — https?://[sub.]domain.tld[/path]
	escapedDomain := regexp.QuoteMeta(domain)
	patterns = append(patterns,
		`(?i)https?://(?:[0-9a-z_](?:[0-9a-z_\-]{0,61}[0-9a-z_])?\.)*`+escapedDomain,
	)

	// Pattern 3: Quoted/bracketed domain references — "sub.domain.tld" or 'sub.domain.tld'
	patterns = append(patterns,
		`(?i)(?:[0-9a-z_](?:[0-9a-z_\-]{0,61}[0-9a-z_])?\.)+`+escapedDomain,
	)

	// Pattern 4: Wildcard pattern — *.domain.tld (in configs, certs)
	patterns = append(patterns,
		`(?i)\*\.(?:[0-9a-z_](?:[0-9a-z_\-]{0,61}[0-9a-z_])?\.)*`+escapedDomain,
	)

	// Compile all patterns
	var compiled []*regexp.Regexp
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, re)
	}

	return &RegexpExtractor{
		patterns: compiled,
		domain:   strings.ToLower(domain),
		extend:   extend,
	}, nil
}

// Extract finds all subdomains in the given content using all patterns.
func (e *RegexpExtractor) Extract(content string, source string) []Subdomain {
	seen := make(map[string]struct{})
	var results []Subdomain

	for _, pattern := range e.patterns {
		matches := pattern.FindAllString(content, -1)
		for _, match := range matches {
			cleaned := CleanSubdomain(match)
			if cleaned == "" {
				continue
			}

			// Must contain the base domain name (for extend mode, partial match is OK)
			if !strings.Contains(cleaned, e.domain) {
				if e.extend {
					baseDomain, _ := splitDomainTLD(e.domain)
					if !strings.Contains(cleaned, baseDomain) {
						continue
					}
				} else {
					continue
				}
			}

			if _, exists := seen[cleaned]; exists {
				continue
			}
			seen[cleaned] = struct{}{}

			results = append(results, Subdomain{
				Value:  cleaned,
				Source: source,
			})
		}
	}

	return results
}

// splitDomainTLD splits a domain into its registered domain and TLD.
// Falls back to simple dot-split if publicsuffix lookup fails.
func splitDomainTLD(domain string) (string, string) {
	// Try publicsuffix for accurate TLD detection (handles co.uk, com.au, etc.)
	tld, _ := publicsuffix.PublicSuffix(domain)
	if tld != "" && strings.HasSuffix(domain, "."+tld) {
		domainWithoutTLD := strings.TrimSuffix(domain, "."+tld)
		parts := strings.Split(domainWithoutTLD, ".")
		return parts[len(parts)-1], tld
	}

	// Fallback: simple split on last dot
	parts := strings.Split(domain, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-2], parts[len(parts)-1]
	}
	return domain, ""
}
