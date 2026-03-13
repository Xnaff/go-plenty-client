# Technology Stack

**Project:** PlentyOne Product Generator
**Researched:** 2026-03-12
**Overall confidence:** MEDIUM -- all recommendations based on training data (cutoff May 2025). Go ecosystem is stable enough that core recommendations are unlikely to have changed, but exact version numbers need verification before `go mod init`.

**Verification note:** WebSearch, WebFetch, and Bash were unavailable during this research session. All version numbers and API details are from training data. Before starting implementation, run `go install` or check GitHub releases to confirm latest versions.

---

## Recommended Stack

### Language & Runtime

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go | 1.22+ | Primary language | Project constraint. Go 1.22 added enhanced routing in `net/http` (method-based routing like `mux.HandleFunc("GET /api/products", handler)`), which reduces dependency on third-party routers. If Go 1.23+ is available, prefer it for iterator support and further stdlib improvements. | HIGH |

### HTTP Server / Web Framework

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `go-chi/chi/v5` | v5.1+ | HTTP router for API + dashboard | Chi is the Go community's preferred lightweight router. It follows `net/http` conventions (uses standard `http.Handler` interface), has excellent middleware support (CORS, logging, auth, rate limiting), and adds URL parameter parsing and route grouping that stdlib still lacks. Chi is NOT a framework -- it's a router that composes with stdlib, which is exactly right for this project. Fiber/Gin use non-standard interfaces that create ecosystem friction. Echo is fine but less popular than chi in 2024-2025 projects. | HIGH |

### CLI Framework

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `spf13/cobra` | v1.8+ | CLI command structure | Cobra is the de facto standard for Go CLIs. Used by kubectl, Hugo, GitHub CLI, Docker. Handles subcommands, flags, help text, shell completion. Mature, extremely well-tested, not going anywhere. Alternatives (urfave/cli, kong) are fine but smaller ecosystem and less documentation. | HIGH |
| `spf13/viper` | v1.18+ | Configuration management | Companion to Cobra. Reads config from files (YAML, JSON, TOML), environment variables, and CLI flags with automatic binding. Supports config file watching for hot reload. Needed for managing API keys, database credentials, PlentyONE OAuth config, AI provider settings. | HIGH |

### Database Access

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `go-sql-driver/mysql` | v1.8+ | MySQL driver | The only serious MySQL driver for Go. Required underneath any higher-level library. | HIGH |
| `sqlc` (code generator) | v1.25+ | SQL-first data access | **Use sqlc, not GORM or sqlx.** sqlc compiles SQL queries into type-safe Go code at build time. You write real SQL, sqlc generates the Go functions and structs. This is the right choice because: (1) the mapping tables are schema-heavy (entity ID mappings, pipeline state, job tracking) where you want full SQL control, (2) compile-time type safety catches errors before runtime, (3) no reflection or runtime overhead like GORM, (4) generated code is trivially debuggable. The project has ~15-20 tables with well-defined schemas -- perfect for sqlc. MySQL support in sqlc is solid (added in v1.17+). | HIGH |
| `golang-migrate/migrate` | v4.17+ | Database migrations | Schema versioning. SQL-based migration files (not Go code migrations). Up/down migrations for every schema change. Integrates with `go-sql-driver/mysql`. Alternatives: goose (also good), but migrate has broader adoption. | HIGH |

### AI Provider Integration

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `sashabaranov/go-openai` | v1.28+ | OpenAI API client | The dominant Go client for OpenAI. Covers Chat Completions, Image Generation (DALL-E), Embeddings. Well-maintained, tracks OpenAI API changes closely. Use this for the primary AI provider. | MEDIUM |
| `anthropics/anthropic-sdk-go` | v0.2+ | Anthropic API client | Anthropic released an official Go SDK in late 2024 / early 2025. Use the official SDK rather than community alternatives. Covers Messages API (Claude). Version number needs verification -- was relatively new at training cutoff. | LOW |
| Custom HTTP client | N/A | Fallback / other providers | For any AI provider without a Go SDK, use a thin custom HTTP client wrapper. The provider abstraction layer should define an interface (`Generator`) that each provider implements, so adding new providers means writing one adapter file. | HIGH |

**Architecture note:** Define a `Generator` interface early:
```go
type Generator interface {
    GenerateText(ctx context.Context, req TextRequest) (TextResponse, error)
    GenerateImage(ctx context.Context, req ImageRequest) (ImageResponse, error)
}
```
Each AI provider (OpenAI, Anthropic, etc.) implements this. The pipeline never calls provider-specific code directly.

### HTTP Client (for PlentyONE API + external APIs)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `net/http` (stdlib) | N/A | HTTP client foundation | Go's stdlib HTTP client is excellent. Do NOT use third-party HTTP clients (resty, go-resty, etc.) -- they add abstraction without value for this use case. Build a thin wrapper around `*http.Client` that adds OAuth token injection, rate limiting, retry, and structured logging. | HIGH |
| `golang.org/x/time/rate` | latest | Rate limiting | Stdlib-adjacent token bucket rate limiter. Used to throttle PlentyONE API calls. Configure with PlentyONE's documented rate limits. Simple, battle-tested, no third-party dependency needed. | HIGH |
| `golang.org/x/oauth2` | latest | OAuth2 token management | Standard Go OAuth2 library. Handles token refresh automatically via `TokenSource` pattern. Wrap PlentyONE's OAuth endpoint with a custom `TokenSource`. Tokens persisted to MySQL for restart survival. | HIGH |

### Background Job Processing

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Custom pipeline engine | N/A | 6-stage pipeline orchestration | **Do NOT use a generic job queue (asynq, machinery, river) for the core pipeline.** The 6-stage pipeline is the heart of this application -- it has strict ordering, per-item state tracking, pause-and-flag semantics, and resumability requirements that are too specific for generic job frameworks. Build a custom state machine with MySQL as the persistence layer. This is ~500-800 lines of purpose-built code that will be far more maintainable than fighting a generic framework's abstractions. | HIGH |
| `errgroup` (`golang.org/x/sync/errgroup`) | latest | Bounded concurrency within pipeline stages | Within a single pipeline stage (e.g., "create 500 categories"), use errgroup with semaphore pattern for bounded concurrent API calls. This gives you `N` concurrent workers (configurable, e.g., 5) with proper error propagation and context cancellation. | HIGH |
| `context` (stdlib) | N/A | Cancellation, timeouts | Standard Go context for cancellation propagation, request timeouts, and deadline management throughout the pipeline. | HIGH |

**Why not generic job queues:**
- **asynq** (Redis-based): Requires Redis as an additional dependency for no benefit. The pipeline is not a queue -- it's an ordered state machine. Redis adds operational complexity.
- **river** (Postgres-based): Wrong database. This project uses MySQL.
- **machinery** (Redis-based): Same Redis concern. Also over-engineered for this use case.
- The pipeline needs: strict stage ordering, per-item state (not per-job), pause/resume, idempotency. These are custom semantics that a custom state machine handles cleanly.

### Web Dashboard

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `a-h/templ` | v0.2.700+ | HTML templating | **Use templ, not Go's `html/template`.** templ is a Go HTML templating language that compiles to Go code, giving you type-safe templates with IDE support (autocomplete, error checking). It's the modern standard for Go web UIs, replacing the untyped `html/template`. Perfect for server-rendered dashboard pages. Version needs verification -- was actively releasing in early 2025. | MEDIUM |
| HTMX | v2.0+ | Dynamic UI interactions | Server-rendered HTML with HTMX for dynamic updates (polling pipeline status, loading data previews, toggling config). No JavaScript framework needed. HTMX + templ is the dominant pattern for Go web dashboards in 2024-2025. Avoids the complexity of React/Vue/Svelte + API layer for what is fundamentally an internal tool dashboard. | HIGH |
| Tailwind CSS | v3.4+ (or v4 if stable) | Styling | Utility-first CSS. Fast to build dashboards without writing custom CSS. Use via standalone CLI binary (no Node.js dependency). Tailwind v4 released in early 2025 but v3.4 is the safe choice. | MEDIUM |
| Alpine.js | v3.14+ | Minimal JS interactivity | For the few cases where HTMX is insufficient (client-side form validation, dropdowns, modals). Tiny (15KB), no build step, pairs perfectly with HTMX. | HIGH |

**Why not a JavaScript SPA:**
- This is an internal tool, not a consumer product. Server-rendered HTML is faster to build, easier to maintain, and has zero frontend build complexity.
- HTMX handles the dynamic parts (status polling, partial page updates) without a full frontend framework.
- One language (Go) for the entire stack reduces cognitive load and deployment complexity.
- If the dashboard later needs to become a SPA (unlikely for this use case), it can be extracted. Starting with SSR is the right default.

### Logging & Observability

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `log/slog` (stdlib) | N/A (Go 1.21+) | Structured logging | Go's stdlib structured logger, added in Go 1.21. Use this, not zerolog/zap. `slog` is the standard now -- third-party loggers add dependency without meaningful benefit for new projects. Supports JSON output, log levels, structured key-value pairs. Add a correlation ID (batch ID, product ID) to every log entry for pipeline traceability. | HIGH |

### Testing

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `testing` (stdlib) | N/A | Unit + integration tests | Go's stdlib testing is sufficient. Do not add testify unless the team specifically wants assertion helpers. Table-driven tests are the Go standard. | HIGH |
| `testcontainers-go` | v0.31+ | Integration testing with MySQL | Spins up real MySQL containers for integration tests. Much better than mocking the database -- tests run against real MySQL. Version needs verification. | MEDIUM |
| `go-sqlmock` | v1.5+ | Unit testing DB queries | For unit tests where you want to test SQL logic without a real database. Complements testcontainers for integration tests. | HIGH |
| `net/http/httptest` (stdlib) | N/A | HTTP handler testing | Stdlib HTTP test utilities. Test API handlers without starting a server. | HIGH |

### Configuration & Secrets

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `spf13/viper` | (see CLI section) | Config file management | YAML config files for non-sensitive settings (pipeline config, concurrency limits, language list). | HIGH |
| Environment variables | N/A | Secrets | API keys (OpenAI, Anthropic, PlentyONE OAuth), MySQL password. Never in config files. Viper reads env vars automatically with `AutomaticEnv()`. | HIGH |
| `.env` file (dev only) | N/A | Local development | Use `joho/godotenv` v1.5+ for loading `.env` files in development. Never in production. Add `.env` to `.gitignore`. | HIGH |

### Build & Development

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `air` | v1.52+ | Hot reload (dev) | Watches Go files and rebuilds on change. Essential for dashboard development. Install via `go install github.com/air-verse/air@latest`. | MEDIUM |
| `Taskfile` (go-task) | v3.37+ | Task runner / Makefile replacement | YAML-based task runner. Replaces Makefile with better syntax, cross-platform support, and dependency management. Define tasks for: build, test, migrate, generate (sqlc + templ), dev server. | MEDIUM |
| `golangci-lint` | v1.59+ | Linting | Meta-linter that runs multiple linters. Configure via `.golangci.yml`. Essential for code quality in a multi-component project. | HIGH |

### Embedding & Static Assets

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `embed` (stdlib) | N/A (Go 1.16+) | Embed static assets, migrations, templates | Embed Tailwind CSS output, SQL migration files, and any static dashboard assets into the binary. Single binary deployment with zero external file dependencies. | HIGH |

---

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Web router | chi | Gin | Gin uses custom `gin.Context` instead of stdlib `http.Handler`. Creates ecosystem friction -- middleware from stdlib-compatible libraries won't work. Performance difference is negligible for this use case. |
| Web router | chi | Fiber | Fiber uses fasthttp, not net/http. Incompatible with Go's entire middleware ecosystem. Optimized for extreme throughput that this project doesn't need. |
| Web router | chi | stdlib only (Go 1.22+) | Go 1.22 added method routing but still lacks route groups, middleware chaining, and URL param parsing. Chi adds these with zero deviation from stdlib patterns. |
| Database | sqlc | GORM | GORM uses reflection, has surprising behaviors (silent zero-value updates, auto-migration footguns), and abstracts away SQL in a project where SQL control matters. The mapping tables have specific query patterns that are clearer as raw SQL. |
| Database | sqlc | sqlx | sqlx is good but still runtime-typed. sqlc catches errors at build time. For a project with ~20 tables and complex JOIN queries across mapping tables, compile-time safety is worth the codegen step. |
| Database | sqlc | Ent (entgo.io) | Ent is a full ORM with code generation. Overkill for this schema. Adds significant complexity for graph-based queries we don't need. Right choice for complex domain models, wrong for mapping/tracking tables. |
| Dashboard | templ + HTMX | React / Next.js | Adds a separate frontend project, Node.js dependency, API design overhead, and build complexity. For an internal dashboard, SSR with HTMX is 3x faster to build and 10x easier to maintain. |
| Dashboard | templ + HTMX | Go html/template + HTMX | Works, but `html/template` has no type safety, no IDE support, and error messages are runtime-only. templ fixes all three with compile-time checking. |
| Job processing | Custom pipeline | asynq | Requires Redis. Adds infrastructure dependency for a job queue pattern that doesn't match the pipeline's ordered state machine semantics. |
| Logging | slog | zerolog / zap | zerolog and zap were the right choices before Go 1.21. Now that slog is in stdlib, there's no reason to add a dependency. slog performance is sufficient. |
| CLI | cobra | urfave/cli | Smaller community, fewer examples, less middleware. cobra + viper integration is seamless. |
| Config | viper | koanf | koanf is a fine modern alternative but less ecosystem support and fewer examples. Viper's cobra integration is a significant advantage. |
| Migrations | golang-migrate | goose | Both are good. migrate has broader adoption and supports more source/database combinations. |
| Task runner | Taskfile | Make | Make works but has arcane syntax, poor Windows support, and no built-in dependency management. Taskfile is cleaner YAML syntax with the same capabilities. |

---

## Architecture-Influencing Stack Decisions

### Single Binary Deployment
The entire stack (API server, dashboard, CLI, migrations) compiles into a single Go binary using `embed`. This means:
- No separate frontend deployment
- No migration runner -- migrations embedded and run on startup or via CLI subcommand
- Docker image is `FROM scratch` + binary + CA certs
- This simplifies deployment enormously for what is likely a single-user or small-team tool

### MySQL as the Only External Dependency
By avoiding Redis-backed job queues, the only infrastructure dependency is MySQL. This keeps operations simple:
- Docker Compose for dev: just MySQL
- Production: one binary + one MySQL instance
- No Redis, no message queues, no separate worker processes

### Code Generation Pipeline (Build Step)
Both sqlc and templ require a code generation step before compilation:
```
sqlc generate    -> generates Go code from SQL queries
templ generate   -> generates Go code from .templ files
go build         -> compiles everything
```
This is handled by Taskfile. Developers run `task generate` or `task build` (which runs generate first). CI/CD does the same.

---

## Installation

```bash
# Initialize module
go mod init github.com/[user]/plentyone

# Core dependencies
go get github.com/go-chi/chi/v5
go get github.com/go-chi/cors
go get github.com/spf13/cobra
go get github.com/spf13/viper
go get github.com/go-sql-driver/mysql
go get github.com/golang-migrate/migrate/v4
go get golang.org/x/oauth2
go get golang.org/x/time
go get golang.org/x/sync

# AI providers
go get github.com/sashabaranov/go-openai
# go get github.com/anthropics/anthropic-sdk-go  # verify package path before install

# Dashboard
go get github.com/a-h/templ
go get github.com/joho/godotenv

# Dev tools (install, not go get)
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/a-h/templ/cmd/templ@latest
go install github.com/air-verse/air@latest
go install github.com/go-task/task/v3/cmd/task@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Testing
go get github.com/testcontainers/testcontainers-go
go get github.com/DATA-DOG/go-sqlmock

# Frontend (no Node.js required)
# Download Tailwind CSS standalone CLI binary from GitHub releases
# Download HTMX and Alpine.js as static files (embed in binary)
```

---

## Version Verification Checklist

Before starting implementation, verify these versions against current releases:

| Package | Verify At | Training Data Version |
|---------|-----------|----------------------|
| Go | go.dev/dl | 1.22+ (1.23 or 1.24 may be current) |
| chi | github.com/go-chi/chi/releases | v5.1.0 |
| cobra | github.com/spf13/cobra/releases | v1.8.x |
| viper | github.com/spf13/viper/releases | v1.18.x or v1.19.x |
| sqlc | github.com/sqlc-dev/sqlc/releases | v1.25.x or v1.26.x |
| templ | github.com/a-h/templ/releases | v0.2.700+ (may have hit v1.0) |
| go-openai | github.com/sashabaranov/go-openai/releases | v1.28+ |
| anthropic-sdk-go | github.com/anthropics/anthropic-sdk-go | v0.2+ (may have new path) |
| golang-migrate | github.com/golang-migrate/migrate/releases | v4.17+ |
| HTMX | htmx.org | v2.0+ |
| Tailwind CSS | tailwindcss.com | v3.4 or v4.x |
| testcontainers-go | github.com/testcontainers/testcontainers-go | v0.31+ |

---

## Sources

All recommendations based on training data (cutoff May 2025). Confidence levels:

- **HIGH confidence items:** chi, cobra, viper, go-sql-driver/mysql, sqlc (MySQL support), golang-migrate, slog, errgroup, embed, HTMX, golangci-lint -- these are well-established in the Go ecosystem with stable APIs and strong adoption. Unlikely to have changed significantly.
- **MEDIUM confidence items:** templ (rapidly evolving, may have hit v1.0 or changed API), sashabaranov/go-openai (tracks fast-moving OpenAI API), Tailwind CSS (v4 transition in progress), testcontainers-go (active development), air, Taskfile -- these are recommended but versions may have shifted.
- **LOW confidence items:** anthropic-sdk-go (was very new at training cutoff, package path and API may have changed), exact version numbers for all packages -- verify before `go mod init`.

**Key verification needed:** The Anthropic Go SDK was in early stages. Before implementing the Anthropic provider adapter, verify: (1) package exists at expected path, (2) API coverage (Messages API), (3) whether it's still the recommended approach vs. raw HTTP client.
