<h1 align="center">github-subdomains</h1>

<h4 align="center">Find subdomains on GitHub.</h4>

<p align="center">
    <img src="https://img.shields.io/badge/go-v1.22+-blue" alt="go badge">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="MIT license badge">
    <img src="https://img.shields.io/badge/version-2.0.0-orange" alt="version badge">
</p>

<p align="center">
    <img src="./preview.png" alt="github-subdomains preview" width="700">
</p>

---

## Description

A modern Go tool that discovers subdomains by searching GitHub repositories. It leverages the GitHub Code Search, Commit Search, and Issue Search APIs to find subdomains exposed in publicly available source code, configuration files, commit messages, and issue discussions.

### Features

- **Multi-source search** - Code, commits, and issues/PRs
- **Advanced search queries** - Searches `.env` files, config files, deploy paths, Terraform/Ansible files, DNS records, and more
- **Priority-based query engine** - High/medium/low priority queries, automatically limited based on available token count
- **Multiple output formats** - Text (one subdomain per line), JSON (array), and JSONL (streaming, one object per line)
- **Checkpoint/resume** - Automatically saves progress every 30 seconds; resume interrupted scans with `--resume`
- **Concurrent processing** - Configurable worker pool with bounded concurrency via `errgroup`
- **Smart token management** - Automatic rotation across multiple tokens, 70-second cooldown on rate-limited tokens, automatic removal of invalid credentials
- **Proxy support** - HTTP and SOCKS5 proxies
- **GitHub Enterprise** - Custom API base URL support
- **Progress tracking** - Real-time progress bar with ETA and found-subdomain counter
- **Early termination** - Automatically stops after 25 consecutive queries that yield no new results
- **Shell completion** - Bash, Zsh, Fish, PowerShell

---

## Install

```
go install github.com/gwen001/github-subdomains@latest
```

Or build from source:

```bash
git clone https://github.com/gwen001/github-subdomains
cd github-subdomains
make build
```

Or download a pre-built binary from the [Releases](https://github.com/gwen001/github-subdomains/releases) page.

---

## Usage

```
$ github-subdomains --help

Usage:
  github-subdomains [flags]
  github-subdomains [command]

Flags:
  -d, --domain string       target domain (required)
  -t, --token string        GitHub token(s): single token, comma-separated, or file path
  -o, --output string       output file (default: <domain>.txt)
  -f, --format string       output format: text, json, jsonl (default "text")
  -e, --extend              extended mode: also search for <prefix>domain variants
  -q, --quick               quick mode: skip language/noise filter refinement
  -c, --concurrency int     max concurrent requests (default 30)
  -k, --kill-on-empty       exit when all tokens are disabled
      --raw                 raw output (subdomains only, no banner/colors)
      --proxy string        HTTP/SOCKS5 proxy URL
      --github-url string   GitHub API base URL (default "https://api.github.com")
      --resume string       resume from checkpoint file
      --sources strings     search sources: code,commits,issues (default [code])
      --timeout duration    HTTP request timeout (default 10s)
  -v, --verbose             verbose debug output
  -h, --help                help for github-subdomains

Commands:
  completion  Generate the autocompletion script for the specified shell
  version     Print version information
```

### Examples

Basic usage:
```bash
github-subdomains -d example.com -t ghp_your_token_here
```

Multi-source search with JSONL streaming output:
```bash
github-subdomains -d example.com --sources code,commits,issues -f jsonl -o results.jsonl
```

Using multiple tokens from a file:
```bash
github-subdomains -d example.com -t .tokens
```

Extended mode with proxy:
```bash
github-subdomains -d example.com -e --proxy socks5://127.0.0.1:1080
```

Quick mode (skip language/noise refinement for faster scans):
```bash
github-subdomains -d example.com -q -t .tokens
```

Resume an interrupted scan:
```bash
github-subdomains -d example.com --resume example.com.checkpoint.json
```

GitHub Enterprise:
```bash
github-subdomains -d corp.internal -t $GHE_TOKEN --github-url https://github.corp.com/api/v3
```

Raw output piped to other tools:
```bash
github-subdomains -d example.com --raw | sort -u | httpx -silent
```

---

## How It Works

### Search Strategies

The tool uses three independent search strategies, each targeting a different GitHub API:

| Strategy | API | What it searches |
|----------|-----|-----------------|
| **Code** | `/search/code` | Source code, config files, `.env` files, Terraform/Ansible files, DNS configs, deploy scripts, CI/CD pipelines, security recon files (`subdomains.txt`, `scope.txt`, etc.) |
| **Commits** | `/search/commits` | Commit messages mentioning the target domain, filtered by keywords like `deploy`, `dns`, `subdomain`, `config`, `ssl`, `migration` |
| **Issues** | `/search/issues` | Issues and pull requests referencing the domain in titles, bodies, or comments |

### Query Priority System

Each strategy generates queries at three priority levels:

- **Priority 1 (High)** - Always executed. Base domain queries with sort/order variations, wildcard patterns, protocol-prefixed searches, org-scoped queries, and security recon file searches.
- **Priority 2 (Medium)** - Executed with 1+ tokens. Filename patterns (`.env`, `nginx.conf`, `docker-compose`, `terraform.tfvars`), file extension filters (`.yml`, `.json`, `.tf`, `.conf`), infrastructure path queries, NOT-filters, and size-range splits.
- **Priority 3 (Low)** - Skipped with a single token to conserve rate limits. Niche filenames (CI configs, monitoring tools, SSL certs), niche extensions (`.hcl`, `.sql`, `.pem`), all path patterns, and subdomain prefix brute-force (50 common prefixes like `api.`, `staging.`, `admin.`, etc.).

### Search Refinement

When a query returns more than 1,000 results (GitHub's result cap), the tool automatically generates refined sub-queries to bypass this limit:

1. **Language splits** - Re-runs the query filtered by each programming language from `languages.txt` (e.g., JavaScript, Python, Go, etc.)
2. **Noise keyword splits** - Re-runs the query with additional keywords from `noise.txt` (e.g., `api`, `internal`, `production`) and built-in defaults (`staging`, `test`, `admin`, `dev`, `sandbox`, `demo`)

This refinement is skipped when using `--quick` mode.

### Result Extraction

For each search result:
1. **Text matches first** - Extracts subdomains from the `text_matches` field returned by the API (no extra API call needed).
2. **Raw content fallback** - If no subdomains are found in text matches, fetches the raw file content and extracts from that.
3. **Cleaning and validation** - Normalizes extracted strings: strips protocols, URL paths, and encoding artifacts; validates against the public suffix list; deduplicates.

---

## Output Formats

### Text (default)

One subdomain per line, flushed immediately (compatible with `tail -f`):

```
api.example.com
staging.example.com
admin.example.com
```

### JSON

A JSON array written on completion:

```json
[
  {
    "subdomain": "api.example.com",
    "source": "https://github.com/user/repo/blob/main/.env",
    "found_at": "2025-01-15T10:30:00Z"
  },
  {
    "subdomain": "staging.example.com",
    "source": "https://github.com/user/repo/blob/main/deploy/config.yml",
    "found_at": "2025-01-15T10:30:05Z"
  }
]
```

### JSONL (streaming)

One JSON object per line, written in real-time as results are discovered:

```json
{"subdomain":"api.example.com","source":"https://github.com/user/repo/blob/main/.env","found_at":"2025-01-15T10:30:00Z"}
{"subdomain":"staging.example.com","source":"https://github.com/user/repo/blob/main/deploy/config.yml","found_at":"2025-01-15T10:30:05Z"}
```

---

## Token Configuration

Provide tokens via:

1. **Command-line flag**: `-t ghp_xxxxx` or `-t token1,token2`
2. **File**: `-t .tokens` (one token per line, blank lines and `#` comments are ignored)
3. **Environment variable**: `export GITHUB_TOKEN=token1,token2`

Supported token formats:
- Classic: 40-character hex (`abcdef0123456789...`)
- Personal access token: `ghp_...` (36 chars after prefix)
- Fine-grained PAT: `github_pat_...` (82 chars after prefix)

### Token Behavior

- **Rotation** - Tokens are cycled round-robin across requests to distribute load.
- **Rate-limit recovery** - When a token hits GitHub's rate limit, it is automatically disabled for a **70-second cooldown** then re-enabled.
- **Bad credential removal** - Tokens that return 401 (unauthorized) are permanently removed from the pool.
- **Kill-on-empty (`-k`)** - When all tokens are disabled (rate-limited or removed), the tool can either wait for recovery (default) or exit immediately with this flag.
- **Token-aware query limiting** - With only 1 token, low-priority queries are automatically skipped to conserve rate limits.

---

## Checkpoint / Resume

Long-running scans are automatically checkpointed to protect against interruptions.

- **Auto-save** - Progress is saved every **30 seconds** to `<domain>.checkpoint.json`.
- **Graceful shutdown** - On `SIGINT` or `SIGTERM`, a final checkpoint is saved before exit.
- **Saved state** includes:
  - Current search query index
  - All processed GitHub URLs (for deduplication)
  - All discovered subdomains
  - Timestamp
- **Resume** - Pass `--resume <domain>.checkpoint.json` to pick up where you left off. The domain in the checkpoint must match the target domain.

---

## Customization Files

### `languages.txt`

A list of programming languages (one per line) used for search refinement when a query exceeds 1,000 results. The tool re-runs the query filtered by each language to bypass GitHub's result cap.

Default languages include: JavaScript, Python, Java, Go, Ruby, PHP, Shell, CSV, Markdown, XML, JSON, Text, CSS, HTML, Perl, Lua, C, C++, C#.

URL-encoded values are supported (e.g., `C%2B%2B` for C++, `C%23` for C#).

### `noise.txt`

Additional keywords appended to search queries during refinement. When a query returns more than 1,000 results, the tool generates sub-queries combining the original query with each noise keyword.

Default file includes: `api`, `private`, `secret`, `internal`, `corp`, `development`, `production`.

Built-in defaults (always included): `staging`, `test`, `admin`, `dev`, `sandbox`, `demo`.

---

## Development

```bash
make test       # Run tests with race detector
make lint       # Run golangci-lint
make build      # Build binary
make cover      # Generate coverage report
make install    # Install to $GOPATH/bin
make fmt        # Format code
make vet        # Run go vet
make release    # Build snapshot release with goreleaser
make clean      # Remove build artifacts
```

### Project Structure

```
cmd/github-subdomains/     CLI entrypoint (Cobra)
internal/
  config/                  Configuration, defaults, token loading, validation
  token/                   Token management with rotation, cooldown, and removal
  github/                  GitHub API client with retries, rate limiting, and proxy support
    search.go              Search strategies (Code, Commit, Issue) and query builders
    client.go              HTTP client, API calls, rate-limit handling
    types.go               Shared types (SearchQuery, SearchItem, SearchResponse)
  extractor/               Regex-based subdomain extraction and normalization
    cleaner.go             URL decoding, protocol stripping, public suffix validation
  dedup/                   Thread-safe string set for deduplication
  output/                  Output writers (Text, JSON, JSONL) and progress bar
  runner/                  Orchestrator: wires all components, manages search lifecycle
  checkpoint/              Save/load scan state for resume capability
languages.txt              Language list for search refinement
noise.txt                  Noise keywords for search refinement
```

---

## License

MIT - See [LICENSE.md](LICENSE.md) for details.

---

Feel free to [open an issue](/../../issues/) if you have any problem with the tool.
