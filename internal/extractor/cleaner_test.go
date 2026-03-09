package extractor

import "testing"

func TestCleanSubdomain(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"basic", "api.example.com", "api.example.com"},
		{"uppercase", "API.Example.COM", "api.example.com"},
		{"leading dot", ".api.example.com", "api.example.com"},
		{"leading slash", "/api.example.com", "api.example.com"},
		{"url encoded 2f prefix", "2fapi.example.com", "api.example.com"},
		{"url encoded 252f prefix", "252fapi.example.com", "api.example.com"},
		{"unicode escape", "\\u002fapi.example.com", "api.example.com"},
		{"empty", "", ""},
		{"single label", "localhost", ""},
		{"too long label", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.example.com", ""},
		{"trailing dot", "api.example.com.", "api.example.com"},
		{"whitespace", "  api.example.com  ", "api.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanSubdomain(tt.input)
			if got != tt.want {
				t.Errorf("CleanSubdomain(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidDomain(t *testing.T) {
	tests := []struct {
		domain string
		valid  bool
	}{
		{"example.com", true},
		{"sub.example.com", true},
		{"a.b.c.d.example.com", true},
		{"example", false},
		{"", false},
		{".com", false},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := isValidDomain(tt.domain)
			if got != tt.valid {
				t.Errorf("isValidDomain(%q) = %v, want %v", tt.domain, got, tt.valid)
			}
		})
	}
}
