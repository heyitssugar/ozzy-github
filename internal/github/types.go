package github

// SearchResponse represents the GitHub Code/Commit/Issue search API response.
type SearchResponse struct {
	Message           string       `json:"message"`
	DocumentationURL  string       `json:"documentation_url"`
	TotalCount        int          `json:"total_count"`
	IncompleteResults bool         `json:"incomplete_results"`
	Items             []SearchItem `json:"items"`
}

// SearchItem represents a single item from code search results.
type SearchItem struct {
	Name       string     `json:"name"`
	Path       string     `json:"path"`
	HTMLURL    string     `json:"html_url"`
	Repository Repository `json:"repository"`
	TextMatches []TextMatch `json:"text_matches,omitempty"`
}

// Repository holds basic repository info from search results.
type Repository struct {
	FullName    string `json:"full_name"`
	HTMLURL     string `json:"html_url"`
	Description string `json:"description"`
}

// TextMatch represents a text match fragment from the API.
type TextMatch struct {
	Fragment string `json:"fragment"`
}

// CommitSearchResponse represents GitHub commit search results.
type CommitSearchResponse struct {
	Message    string         `json:"message"`
	TotalCount int            `json:"total_count"`
	Items      []CommitItem   `json:"items"`
}

// CommitItem represents a single commit from search results.
type CommitItem struct {
	HTMLURL string       `json:"html_url"`
	Commit  CommitDetail `json:"commit"`
}

// CommitDetail holds commit message and metadata.
type CommitDetail struct {
	Message string `json:"message"`
	URL     string `json:"url"`
}

// IssueSearchResponse represents GitHub issue/PR search results.
type IssueSearchResponse struct {
	Message    string      `json:"message"`
	TotalCount int         `json:"total_count"`
	Items      []IssueItem `json:"items"`
}

// IssueItem represents a single issue/PR from search results.
type IssueItem struct {
	HTMLURL string `json:"html_url"`
	Title   string `json:"title"`
	Body    string `json:"body"`
}

// SourceType identifies which GitHub search API endpoint to use.
type SourceType int

const (
	SourceCode    SourceType = iota // /search/code
	SourceCommit                    // /search/commits
	SourceIssue                     // /search/issues
)

// Priority controls search query execution order and token-aware filtering.
type Priority int

const (
	PriorityHigh   Priority = 1 // Always run — base queries, most productive
	PriorityMedium Priority = 2 // Run with 1+ tokens — common filenames, extensions
	PriorityLow    Priority = 3 // Run only with 2+ tokens or when no early termination
)

// SearchQuery represents a single search to execute against the GitHub API.
type SearchQuery struct {
	Keyword    string
	Sort       string
	Order      string
	Language   string
	Noise      []string
	Signature  string
	SourceType SourceType
	Priority   Priority
}

// SearchOptions controls how search queries are built.
type SearchOptions struct {
	Languages  []string
	Noise      []string
	QuickMode  bool
	Extend     bool
	TokenCount int // Number of available tokens — controls query volume
}
