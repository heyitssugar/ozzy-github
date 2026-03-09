package github

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// SearchStrategy defines how to build and execute searches against a GitHub API endpoint.
type SearchStrategy interface {
	Name() string
	BuildQueries(encodedDomain string, opts SearchOptions) []SearchQuery
}

// CodeSearchStrategy searches GitHub code repositories with advanced query patterns.
type CodeSearchStrategy struct{}

func (s *CodeSearchStrategy) Name() string { return "code" }

func (s *CodeSearchStrategy) BuildQueries(encodedDomain string, opts SearchOptions) []SearchQuery {
	rawDomain := strings.TrimPrefix(strings.TrimSuffix(encodedDomain, "%22"), "%22")
	unquotedDomain := url.QueryEscape(rawDomain)

	var queries []SearchQuery
	add := func(keyword string, sort, order string, priority Priority) {
		q := SearchQuery{
			Keyword:    keyword,
			Sort:       sort,
			Order:      order,
			SourceType: SourceCode,
			Priority:   priority,
		}
		q.Signature = GenerateSignature(q)
		queries = append(queries, q)
	}

	// ═══ PRIORITY 1 (HIGH) — Always run, most productive ═══

	// Base queries with sort/order variations (bypass 1000-result cap)
	add(encodedDomain, "indexed", "desc", PriorityHigh)
	add(encodedDomain, "indexed", "asc", PriorityHigh)
	add(encodedDomain, "", "desc", PriorityHigh) // best-match

	// Unquoted domain — catches partial matches
	add(unquotedDomain, "indexed", "desc", PriorityHigh)
	add(unquotedDomain, "indexed", "asc", PriorityHigh)

	// Wildcard/protocol/org — high-value specialized queries
	wildcardDomain := url.QueryEscape("*." + rawDomain)
	add("%22"+wildcardDomain+"%22", "indexed", "desc", PriorityHigh)
	for _, proto := range []string{"https://", "http://"} {
		add("%22"+url.QueryEscape(proto+rawDomain)+"%22", "indexed", "desc", PriorityHigh)
	}
	add(fmt.Sprintf("%s+org:%s", encodedDomain, strings.Split(rawDomain, ".")[0]), "indexed", "desc", PriorityHigh)

	// Security recon files — bug bounty data goldmine
	for _, sf := range []string{"subdomains.txt", "domains.txt", "scope.txt", "hosts.txt", "urls.txt", "alive.txt", "resolved.txt", "httpx.txt"} {
		add(fmt.Sprintf("%s+filename:%s", unquotedDomain, sf), "indexed", "desc", PriorityHigh)
	}

	// Content-type qualifiers
	for _, in := range []string{"file", "path"} {
		add(fmt.Sprintf("%s+in:%s", encodedDomain, in), "indexed", "desc", PriorityHigh)
	}

	// ═══ PRIORITY 2 (MEDIUM) — Run with 1+ tokens ═══

	// High-value filename patterns
	for _, fn := range []string{
		".env", "config", "hosts", "dns", "subdomains", "domains", "vhosts",
		"nginx.conf", "docker-compose", "terraform.tfvars", "inventory",
		"settings", ".env.production", ".env.staging",
	} {
		add(fmt.Sprintf("%s+filename:%s", encodedDomain, fn), "indexed", "desc", PriorityMedium)
	}

	// High-value extensions
	for _, ext := range []string{"yml", "yaml", "json", "xml", "toml", "conf", "cfg", "env", "tf", "ini"} {
		add(fmt.Sprintf("%s+extension:%s", encodedDomain, ext), "indexed", "desc", PriorityMedium)
	}

	// Key infrastructure paths
	for _, p := range []string{"deploy", "infrastructure", "terraform", "k8s", "nginx", "docker", "config", "dns", "ssl", "scripts"} {
		add(fmt.Sprintf("%s+path:%s", encodedDomain, p), "indexed", "desc", PriorityMedium)
	}

	// NOT-filters — surface rare results
	for _, nf := range []string{"NOT+test", "NOT+example", "NOT+sample"} {
		add(fmt.Sprintf("%s+%s", encodedDomain, nf), "indexed", "desc", PriorityMedium)
	}

	// Size ranges — different files, different results
	for _, sr := range []string{"size:<1000", "size:1000..10000", "size:>10000"} {
		add(fmt.Sprintf("%s+%s", encodedDomain, sr), "indexed", "desc", PriorityMedium)
	}

	// Repo-scoped
	domainParts := strings.Split(rawDomain, ".")
	if len(domainParts) >= 2 {
		add(fmt.Sprintf("%s+repo:%s/*", encodedDomain, domainParts[0]), "indexed", "desc", PriorityMedium)
	}

	// ═══ PRIORITY 3 (LOW) — Skip with few tokens or when dry ═══

	// Niche filename patterns
	for _, fn := range []string{
		".env.development", "configuration", "named.conf",
		"httpd.conf", "apache", "Caddyfile", "haproxy.cfg", "traefik",
		"variables.tf", "main.tf", "ansible", "playbook",
		"Dockerfile", "Vagrantfile", "Procfile",
		".gitlab-ci.yml", ".travis.yml", "Jenkinsfile", "buildspec.yml", "cloudbuild.yaml",
		"ssl", "cert", "certificate", "tls",
		"package.json", "requirements.txt", "Gemfile", "go.mod",
		"prometheus", "grafana", "alertmanager", "nagios", "zabbix", "datadog",
		"cloudformation", "serverless", "sam-template",
		"readme", "CHANGELOG", "CONTRIBUTING",
		"recon", "bugbounty", "scope", "zones",
	} {
		add(fmt.Sprintf("%s+filename:%s", encodedDomain, fn), "indexed", "desc", PriorityLow)
	}

	// Niche extensions
	for _, ext := range []string{
		"hcl", "properties", "sh", "bash", "zsh",
		"rb", "py", "js", "ts", "go", "php",
		"md", "txt", "csv", "log",
		"sql", "bak", "old", "orig", "htaccess", "swp", "pem", "crt", "key",
	} {
		add(fmt.Sprintf("%s+extension:%s", encodedDomain, ext), "indexed", "desc", PriorityLow)
	}

	// All path patterns
	for _, p := range []string{
		"deployment", "infra", "ansible", "puppet", "chef", "salt",
		"kubernetes", "helm", "charts",
		"apache", "httpd", "caddy", "haproxy", "traefik",
		"containers", "compose",
		"ci", "cd", ".github", ".gitlab", ".circleci", "jenkins",
		"aws", "gcp", "azure", "cloudformation", "serverless",
		"zones", "networking", "proxy", "cdn", "certs",
		"configs", "configuration", "conf", "env", "environments", "envs",
		"tools", "bin", "utils",
		"monitoring", "alerting", "observability",
		"docs", "documentation", "wiki",
	} {
		add(fmt.Sprintf("%s+path:%s", encodedDomain, p), "indexed", "desc", PriorityLow)
	}

	// Remaining NOT-filters
	for _, nf := range []string{"NOT+mock", "NOT+demo", "NOT+template", "NOT+tutorial"} {
		add(fmt.Sprintf("%s+%s", encodedDomain, nf), "indexed", "desc", PriorityLow)
	}

	// Subdomain prefix searches (50 queries — expensive)
	if !opts.QuickMode {
		for _, prefix := range []string{
			"api", "staging", "dev", "admin", "mail", "smtp", "ftp",
			"vpn", "cdn", "app", "portal", "dashboard", "internal",
			"test", "beta", "demo", "sandbox", "jenkins", "gitlab",
			"jira", "confluence", "grafana", "kibana", "elastic",
			"prometheus", "sentry", "auth", "sso", "login", "oauth",
			"ws", "wss", "grpc", "graphql", "rest",
			"db", "database", "mysql", "postgres", "redis", "mongo",
			"s3", "storage", "assets", "static", "media", "images",
			"docs", "wiki", "help", "support", "status",
		} {
			prefixDomain := url.QueryEscape(prefix + "." + rawDomain)
			add("%22"+prefixDomain+"%22", "indexed", "desc", PriorityLow)
		}
	}

	return queries
}

// CommitSearchStrategy searches GitHub commit messages with multiple query variations.
type CommitSearchStrategy struct{}

func (s *CommitSearchStrategy) Name() string { return "commits" }

func (s *CommitSearchStrategy) BuildQueries(encodedDomain string, opts SearchOptions) []SearchQuery {
	rawDomain := strings.TrimPrefix(strings.TrimSuffix(encodedDomain, "%22"), "%22")
	unquotedDomain := url.QueryEscape(rawDomain)

	var queries []SearchQuery
	add := func(keyword string, sort, order string, priority Priority) {
		q := SearchQuery{
			Keyword:    keyword,
			Sort:       sort,
			Order:      order,
			SourceType: SourceCommit,
			Priority:   priority,
		}
		q.Signature = GenerateSignature(q)
		queries = append(queries, q)
	}

	// High: Base sort/order variations
	for _, so := range []struct{ sort, order string }{
		{"author-date", "desc"}, {"author-date", "asc"},
		{"committer-date", "desc"}, {"committer-date", "asc"},
	} {
		add(encodedDomain, so.sort, so.order, PriorityHigh)
		add(unquotedDomain, so.sort, so.order, PriorityMedium)
	}

	// Medium: Top commit keywords
	for _, kw := range []string{"deploy", "dns", "subdomain", "config", "domain", "ssl", "url", "host"} {
		add(fmt.Sprintf("%s+%s", encodedDomain, kw), "author-date", "desc", PriorityMedium)
	}

	// Low: Remaining commit keywords
	for _, kw := range []string{
		"migration", "certificate", "nginx", "apache", "endpoint", "server", "infra",
		"add", "update", "fix", "change", "move",
		"redirect", "proxy", "vhost", "cname", "record",
		"production", "staging", "internal", "api",
	} {
		add(fmt.Sprintf("%s+%s", encodedDomain, kw), "author-date", "desc", PriorityLow)
	}

	// High: Org-scoped
	if len(rawDomain) > 0 {
		add(fmt.Sprintf("%s+org:%s", encodedDomain, strings.Split(rawDomain, ".")[0]), "author-date", "desc", PriorityHigh)
	}

	return queries
}

// IssueSearchStrategy searches GitHub issues and pull requests with expanded queries.
type IssueSearchStrategy struct{}

func (s *IssueSearchStrategy) Name() string { return "issues" }

func (s *IssueSearchStrategy) BuildQueries(encodedDomain string, opts SearchOptions) []SearchQuery {
	rawDomain := strings.TrimPrefix(strings.TrimSuffix(encodedDomain, "%22"), "%22")
	unquotedDomain := url.QueryEscape(rawDomain)
	_ = unquotedDomain

	var queries []SearchQuery
	add := func(keyword string, sort, order string, priority Priority) {
		q := SearchQuery{
			Keyword:    keyword,
			Sort:       sort,
			Order:      order,
			SourceType: SourceIssue,
			Priority:   priority,
		}
		q.Signature = GenerateSignature(q)
		queries = append(queries, q)
	}

	// High: Base sort/order variations (quoted only — most relevant)
	for _, so := range []struct{ sort, order string }{
		{"created", "desc"}, {"updated", "desc"},
	} {
		add(encodedDomain, so.sort, so.order, PriorityHigh)
	}

	// Medium: Additional sort combos
	for _, so := range []struct{ sort, order string }{
		{"created", "asc"}, {"comments", "desc"},
	} {
		add(encodedDomain, so.sort, so.order, PriorityMedium)
		add(unquotedDomain, so.sort, so.order, PriorityLow)
	}
	// Unquoted for high-priority sort combos
	add(unquotedDomain, "created", "desc", PriorityMedium)
	add(unquotedDomain, "updated", "desc", PriorityMedium)

	// Medium: Top issue keywords
	for _, kw := range []string{"subdomain", "dns", "domain", "url", "host", "security", "scope", "bug", "vulnerability"} {
		add(fmt.Sprintf("%s+%s", encodedDomain, kw), "created", "desc", PriorityMedium)
	}

	// Low: Remaining issue keywords
	for _, kw := range []string{
		"endpoint", "certificate", "ssl", "target", "asset", "recon",
		"redirect", "cors", "csp", "xss",
		"api", "internal", "staging", "production",
		"migration", "deploy", "server", "config",
	} {
		add(fmt.Sprintf("%s+%s", encodedDomain, kw), "created", "desc", PriorityLow)
	}

	// Medium: in: qualifiers and type filters
	for _, in := range []string{"title", "body", "comments"} {
		add(fmt.Sprintf("%s+in:%s", encodedDomain, in), "created", "desc", PriorityMedium)
	}
	for _, typ := range []string{"issue", "pr"} {
		add(fmt.Sprintf("%s+type:%s", encodedDomain, typ), "created", "desc", PriorityLow)
	}

	return queries
}

// GetStrategies returns search strategies for the given source names.
func GetStrategies(sources []string) []SearchStrategy {
	var strategies []SearchStrategy
	for _, src := range sources {
		switch src {
		case "code":
			strategies = append(strategies, &CodeSearchStrategy{})
		case "commits":
			strategies = append(strategies, &CommitSearchStrategy{})
		case "issues":
			strategies = append(strategies, &IssueSearchStrategy{})
		}
	}
	return strategies
}

// BuildLanguageQueries generates additional queries filtered by programming language.
func BuildLanguageQueries(base SearchQuery, languages []string) []SearchQuery {
	var queries []SearchQuery
	for _, lang := range languages {
		q := SearchQuery{
			Keyword:    base.Keyword,
			Sort:       base.Sort,
			Order:      base.Order,
			Language:   lang,
			SourceType: base.SourceType,
			Priority:   base.Priority,
		}
		q.Signature = GenerateSignature(q)
		queries = append(queries, q)
	}
	return queries
}

// BuildNoiseQueries generates additional queries with noise keywords.
func BuildNoiseQueries(base SearchQuery, noiseKeywords []string) []SearchQuery {
	var queries []SearchQuery
	for _, noise := range noiseKeywords {
		if contains(base.Noise, noise) {
			continue
		}
		q := SearchQuery{
			Keyword:    base.Keyword,
			Sort:       base.Sort,
			Order:      base.Order,
			Language:   base.Language,
			Noise:      append(append([]string{}, base.Noise...), noise),
			SourceType: base.SourceType,
			Priority:   base.Priority,
		}
		q.Signature = GenerateSignature(q)
		queries = append(queries, q)
	}
	return queries
}

// GenerateSignature creates a unique hash for a search query to prevent duplicates.
func GenerateSignature(q SearchQuery) string {
	parts := []string{q.Keyword, q.Language, q.Sort, q.Order, fmt.Sprintf("%d", q.SourceType)}
	sorted := make([]string, len(q.Noise))
	copy(sorted, q.Noise)
	sort.Strings(sorted)
	parts = append(parts, sorted...)

	hash := sha256.Sum256([]byte(strings.Join(parts, "||")))
	return fmt.Sprintf("%x", hash[:8])
}

// EncodeDomain URL-encodes and quotes a domain for GitHub search.
func EncodeDomain(domain string) string {
	encoded := url.QueryEscape(domain)
	encoded = strings.ReplaceAll(encoded, "-", "%2D")
	return "%22" + encoded + "%22"
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
