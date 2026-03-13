# Architecture Patterns

**Domain:** Go-based API client service with multi-stage pipeline, web dashboard, CLI, and MySQL
**Researched:** 2026-03-12
**Overall confidence:** HIGH (Go project structure patterns are well-established and stable)

## Recommended Architecture

A **layered service architecture** with clear separation between entry points (CLI, HTTP server), business logic (pipeline orchestrator, data generation), infrastructure (API clients, database), and shared domain types.

The system has three entry points sharing a common core:

```
                   +----------+    +-----------+
                   |   CLI    |    | Dashboard |
                   | (cobra)  |    |  (HTTP)   |
                   +----+-----+    +-----+-----+
                        |               |
                        v               v
                +-------+---------------+--------+
                |        Application Layer        |
                |  (Pipeline Orchestrator, Jobs)  |
                +-------+---------------+--------+
                        |               |
              +---------+-----+ +------+--------+
              | Data Sources  | | PlentyONE API |
              | (AI, DBs,    | | Client        |
              | Stock Images) | | (OAuth, Rate  |
              +---------+-----+ |  Limiting)    |
                        |       +------+--------+
                        |              |
                +-------+--------------+--------+
                |       MySQL (State Store)     |
                |  Mappings, Pipeline State,    |
                |  Generated Data, Config       |
                +-------------------------------+
```

### Project Layout

Use the **standard Go project layout** adapted for this service. This is not the controversial `golang-standards/project-layout` -- this follows the pragmatic patterns used by mature Go projects like Kubernetes, Docker, and HashiCorp tools.

```
plentyone/
  cmd/
    server/             # HTTP server + pipeline runner entry point
      main.go
    cli/                # CLI tool entry point
      main.go
  internal/
    app/                # Application-level wiring, config loading
      app.go            # Shared app setup (DB, clients, services)
      config.go         # Configuration struct + loading
    pipeline/           # Pipeline orchestrator (the heart)
      orchestrator.go   # Runs the 6-stage pipeline
      stage.go          # Stage interface + base implementation
      stages/
        categories.go   # Stage 1: Create categories
        attributes.go   # Stage 2: Create attributes/properties
        products.go     # Stage 3: Create parent products
        variations.go   # Stage 4: Create variations
        properties.go   # Stage 5: Connect properties + upload images
        text.go         # Stage 6: Multilingual text
    plentyapi/          # PlentyONE REST API client
      client.go         # HTTP client with OAuth, rate limiting
      auth.go           # OAuth token management + refresh
      ratelimit.go      # Rate limiter
      categories.go     # Category API methods
      products.go       # Product API methods
      variations.go     # Variation API methods
      attributes.go     # Attribute/property API methods
      images.go         # Image upload API methods
      text.go           # Multilingual text API methods
    datasource/         # Data generation + sourcing
      provider.go       # Provider interface
      ai/               # AI provider implementations
        provider.go     # Common AI provider interface
        openai.go       # OpenAI implementation
        anthropic.go    # Anthropic implementation
      openfoodfacts/    # Open Food Facts client
      wikidata/         # Wikidata SPARQL client
      images/           # Stock image APIs (Unsplash, Pexels, Pixabay)
    dashboard/          # Web dashboard HTTP handlers
      server.go         # HTTP server setup, routes
      handlers/         # Route handlers
        pipeline.go     # Pipeline status/control
        data.go         # Data preview
        config.go       # Configuration UI
        mappings.go     # Mapping overview
      templates/        # HTML templates (Go html/template)
      static/           # CSS, JS, static assets
    storage/            # MySQL data access layer
      mysql.go          # Connection setup, migrations
      mapping.go        # Entity mapping CRUD (local ID <-> PlentyONE ID)
      pipeline.go       # Pipeline state persistence
      product.go        # Generated product data storage
      job.go            # Job tracking
    domain/             # Shared domain types (no dependencies)
      product.go        # Product, Variation, Category types
      mapping.go        # Mapping types
      pipeline.go       # Pipeline state, stage status enums
      job.go            # Job types
      errors.go         # Domain-specific error types
    job/                # Job management
      manager.go        # Create, track, pause, resume jobs
      worker.go         # Background job execution
  migrations/           # SQL migration files
    001_initial.up.sql
    001_initial.down.sql
  configs/              # Default config files
    config.example.yaml
  web/                  # Frontend assets (if using separate build)
    static/
    templates/
```

**Key layout decisions:**

- `internal/` for everything -- nothing in this project needs to be importable by external Go code. This is a service, not a library.
- `cmd/server/` and `cmd/cli/` as separate binaries sharing `internal/` packages. Both import and wire the same core services.
- `internal/domain/` has ZERO imports from other internal packages. It defines shared types only. Every other package imports domain; domain imports nothing.
- `internal/pipeline/` is the brain. It orchestrates stages, manages state transitions, handles pause-and-flag.
- `internal/plentyapi/` is a pure API client. It knows nothing about pipelines or jobs. It makes HTTP calls and returns typed responses.
- `internal/storage/` is a pure data access layer. Repository pattern. It knows nothing about pipelines or API clients.

### Component Boundaries

| Component | Responsibility | Imports From | Imported By |
|-----------|---------------|--------------|-------------|
| `domain` | Shared types, enums, error types | stdlib only | Everything |
| `storage` | MySQL CRUD, migrations, connection pooling | `domain` | `pipeline`, `job`, `dashboard`, `app` |
| `plentyapi` | PlentyONE REST API calls, OAuth, rate limiting | `domain` | `pipeline`, `app` |
| `datasource` | AI text/image generation, public DB queries, stock images | `domain` | `pipeline`, `app` |
| `pipeline` | 6-stage orchestration, state machine, pause-and-flag | `domain`, `storage`, `plentyapi`, `datasource` | `job`, `dashboard`, `app` |
| `job` | Background job lifecycle, scheduling, concurrency | `domain`, `storage`, `pipeline` | `dashboard`, CLI handlers, `app` |
| `dashboard` | HTTP handlers, templates, SSE for live updates | `domain`, `storage`, `pipeline`, `job` | `cmd/server` |
| `app` | Wiring, config, dependency injection | Everything in `internal/` | `cmd/server`, `cmd/cli` |

**Dependency rule:** Dependencies flow inward. `domain` is the innermost layer with no dependencies. `app` is the outermost layer that wires everything together. No circular imports.

### Data Flow

#### Pipeline Execution Flow (the critical path)

```
1. User triggers job via CLI or Dashboard
        |
        v
2. Job Manager creates Job record in MySQL
   Sets status = "pending"
        |
        v
3. Job Worker picks up job, creates Pipeline Run
   Pipeline Orchestrator begins stage execution
        |
        v
4. FOR EACH STAGE (1 through 6):
   a. Orchestrator checks: "Has this stage already completed for this run?"
      YES -> skip to next stage
      NO  -> continue
   b. Stage fetches required input:
      - Stage 1 (Categories): Reads generated product data from DB
      - Stage 2 (Attributes): Reads category IDs from mapping table
      - Stage 3 (Products): Reads category + attribute IDs from mappings
      - Stage 4 (Variations): Reads product IDs from mappings
      - Stage 5 (Properties/Images): Reads variation IDs from mappings
      - Stage 6 (Text): Reads variation IDs from mappings
   c. Stage calls PlentyONE API client
      - API client handles OAuth token (refresh if expired)
      - API client handles rate limiting (backoff + retry)
   d. Stage receives API response with PlentyONE IDs
   e. Stage writes mapping to MySQL: (local_id, plenty_id, entity_type, stage)
   f. Stage updates pipeline state in MySQL: stage_status = "completed"
        |
        v
   ON ERROR at any point in (b-f):
   - Stage sets status = "paused_with_error"
   - Records error details + the specific entity that failed
   - Pipeline halts (does NOT continue to next stage)
   - User reviews error in dashboard/CLI
   - User can: fix data and resume, skip entity, or abort
        |
        v
5. All stages complete -> Job status = "completed"
   Dashboard shows green, all mappings visible
```

#### Data Generation Flow (feeds into pipeline)

```
1. User configures job:
   - Product type / niche / category
   - Quantity desired
   - AI provider selection
   - Languages for text
   - Image source preference
        |
        v
2. Data Source Manager orchestrates generation:
   a. Query public databases for base product data
      (Open Food Facts for food, Wikidata for other domains)
   b. AI provider enriches / generates missing fields:
      - Product names (multilingual)
      - Descriptions (multilingual, 5 languages)
      - Property values
      - Variation attributes
   c. Image sourcing:
      - Stock image APIs (Unsplash, Pexels, Pixabay)
      - OR AI image generation
   d. All generated data stored in MySQL
      (products, variations, texts, image URLs)
        |
        v
3. Generated data is now input for the Pipeline stages
```

#### Request Flow: Dashboard

```
Browser -> HTTP Server -> Router -> Handler -> Service Layer -> MySQL/Pipeline
                                                    |
                                                    v
                                              HTML Template
                                                    |
                                                    v
                                              HTTP Response

For live pipeline updates:
Browser <--SSE-- HTTP Server <-- Pipeline Event Channel
```

## Patterns to Follow

### Pattern 1: Stage Interface with State Machine

The pipeline is the core abstraction. Each stage implements a common interface. The orchestrator manages transitions.

**What:** Define each pipeline stage as a struct implementing a `Stage` interface. The orchestrator iterates stages in order, checking state, executing, and persisting results.

**When:** Always -- this is the foundational pattern for the entire system.

**Example:**

```go
package pipeline

import "context"

// StageStatus represents the state of a pipeline stage for a given run.
type StageStatus string

const (
    StageStatusPending   StageStatus = "pending"
    StageStatusRunning   StageStatus = "running"
    StageStatusCompleted StageStatus = "completed"
    StageStatusFailed    StageStatus = "failed"
    StageStatusSkipped   StageStatus = "skipped"
)

// Stage is implemented by each of the 6 pipeline stages.
type Stage interface {
    // Name returns a human-readable name for the stage.
    Name() string

    // Order returns the execution order (1-6).
    Order() int

    // Execute runs the stage for a batch of entities.
    // It should be idempotent: if called again after partial completion,
    // it should skip already-processed entities.
    // Returns the number of entities processed and any error.
    Execute(ctx context.Context, runID int64) (processed int, err error)
}

// Orchestrator runs stages in order, managing state transitions.
type Orchestrator struct {
    stages  []Stage
    store   StateStore
    events  chan Event
}

func (o *Orchestrator) Run(ctx context.Context, runID int64) error {
    for _, stage := range o.stages {
        status, err := o.store.GetStageStatus(ctx, runID, stage.Name())
        if err != nil {
            return fmt.Errorf("checking stage %s: %w", stage.Name(), err)
        }
        if status == StageStatusCompleted || status == StageStatusSkipped {
            continue // Resume support: skip completed stages
        }

        o.store.SetStageStatus(ctx, runID, stage.Name(), StageStatusRunning)
        o.events <- Event{RunID: runID, Stage: stage.Name(), Status: StageStatusRunning}

        processed, err := stage.Execute(ctx, runID)
        if err != nil {
            o.store.SetStageStatus(ctx, runID, stage.Name(), StageStatusFailed)
            o.store.RecordError(ctx, runID, stage.Name(), err)
            o.events <- Event{RunID: runID, Stage: stage.Name(), Status: StageStatusFailed, Err: err}
            return fmt.Errorf("stage %s failed after %d entities: %w", stage.Name(), processed, err)
        }

        o.store.SetStageStatus(ctx, runID, stage.Name(), StageStatusCompleted)
        o.events <- Event{RunID: runID, Stage: stage.Name(), Status: StageStatusCompleted, Processed: processed}
    }
    return nil
}
```

### Pattern 2: Repository Pattern for MySQL Access

**What:** Each entity type gets a repository struct that encapsulates all SQL. Business logic never writes raw SQL.

**When:** All database access.

```go
package storage

// MappingRepo handles local_id <-> plenty_id mappings.
type MappingRepo struct {
    db *sql.DB
}

func (r *MappingRepo) SaveMapping(ctx context.Context, m domain.Mapping) error {
    _, err := r.db.ExecContext(ctx,
        `INSERT INTO entity_mappings (local_id, plenty_id, entity_type, stage, run_id, created_at)
         VALUES (?, ?, ?, ?, ?, NOW())
         ON DUPLICATE KEY UPDATE plenty_id = VALUES(plenty_id)`,
        m.LocalID, m.PlentyID, m.EntityType, m.Stage, m.RunID,
    )
    return err
}

func (r *MappingRepo) GetPlentyID(ctx context.Context, localID int64, entityType string) (int64, error) {
    var plentyID int64
    err := r.db.QueryRowContext(ctx,
        `SELECT plenty_id FROM entity_mappings WHERE local_id = ? AND entity_type = ?`,
        localID, entityType,
    ).Scan(&plentyID)
    return plentyID, err
}
```

### Pattern 3: Provider Interface for Data Sources

**What:** Abstract AI providers and public databases behind interfaces. Concrete implementations are swappable.

**When:** All data sourcing -- AI text, AI images, public databases, stock images.

```go
package datasource

// TextGenerator generates product text in multiple languages.
type TextGenerator interface {
    GenerateProductName(ctx context.Context, input ProductInput, lang string) (string, error)
    GenerateDescription(ctx context.Context, input ProductInput, lang string) (string, error)
    GenerateProperties(ctx context.Context, input ProductInput) ([]Property, error)
}

// ImageSource provides product images.
type ImageSource interface {
    FindImages(ctx context.Context, query string, count int) ([]Image, error)
}

// ProductDatabase provides base product data from public sources.
type ProductDatabase interface {
    Search(ctx context.Context, query string, limit int) ([]RawProduct, error)
    GetByID(ctx context.Context, id string) (*RawProduct, error)
}
```

### Pattern 4: OAuth Client with Token Refresh Middleware

**What:** Wrap `http.Client` with a transport that handles OAuth token lifecycle transparently. API client code never thinks about tokens.

**When:** All PlentyONE API calls.

```go
package plentyapi

// AuthTransport is an http.RoundTripper that injects OAuth tokens
// and refreshes them when expired.
type AuthTransport struct {
    Base         http.RoundTripper
    TokenStore   TokenStore
    ClientID     string
    ClientSecret string
    TokenURL     string
    mu           sync.Mutex
}

func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    token, err := t.TokenStore.GetToken(req.Context())
    if err != nil || token.Expired() {
        token, err = t.refreshToken(req.Context())
        if err != nil {
            return nil, fmt.Errorf("refreshing token: %w", err)
        }
    }
    req.Header.Set("Authorization", "Bearer "+token.AccessToken)

    resp, err := t.base().RoundTrip(req)
    if err != nil {
        return nil, err
    }

    // If 401, try one refresh and retry
    if resp.StatusCode == http.StatusUnauthorized {
        resp.Body.Close()
        token, err = t.refreshToken(req.Context())
        if err != nil {
            return nil, fmt.Errorf("refreshing token after 401: %w", err)
        }
        req.Header.Set("Authorization", "Bearer "+token.AccessToken)
        return t.base().RoundTrip(req)
    }

    return resp, nil
}
```

### Pattern 5: Functional Options for Configuration

**What:** Use functional options pattern for constructing complex objects with sensible defaults.

**When:** Constructing API clients, pipeline orchestrators, server instances.

```go
package plentyapi

type ClientOption func(*Client)

func WithRateLimit(rps float64) ClientOption {
    return func(c *Client) {
        c.rateLimiter = rate.NewLimiter(rate.Limit(rps), 1)
    }
}

func WithTimeout(d time.Duration) ClientOption {
    return func(c *Client) {
        c.httpClient.Timeout = d
    }
}

func NewClient(baseURL string, opts ...ClientOption) *Client {
    c := &Client{
        baseURL:     baseURL,
        httpClient:  &http.Client{Timeout: 30 * time.Second},
        rateLimiter: rate.NewLimiter(2, 1), // Default: 2 req/sec
    }
    for _, opt := range opts {
        opt(c)
    }
    return c
}
```

### Pattern 6: Event Channel for Live Dashboard Updates

**What:** Pipeline orchestrator emits events to a channel. Dashboard uses Server-Sent Events (SSE) to push updates to the browser.

**When:** Pipeline status updates to dashboard.

```go
// In pipeline orchestrator:
type Event struct {
    RunID     int64
    Stage     string
    Status    StageStatus
    Processed int
    Total     int
    Err       error
    Timestamp time.Time
}

// In dashboard SSE handler:
func (h *Handler) StreamEvents(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")

    for {
        select {
        case event := <-h.events:
            data, _ := json.Marshal(event)
            fmt.Fprintf(w, "data: %s\n\n", data)
            flusher.Flush()
        case <-r.Context().Done():
            return
        }
    }
}
```

## Anti-Patterns to Avoid

### Anti-Pattern 1: God Pipeline Function

**What:** One massive function that handles all 6 stages sequentially with nested if/else for error handling.

**Why bad:** Impossible to resume from a specific stage. Cannot test stages independently. Cannot add/reorder stages without rewriting everything. Error handling becomes spaghetti.

**Instead:** Stage interface pattern (Pattern 1 above). Each stage is independent, testable, and the orchestrator manages sequencing.

### Anti-Pattern 2: API Client Knowing About Pipeline State

**What:** The PlentyONE API client package imports pipeline or storage packages to save mappings or update state.

**Why bad:** Circular dependency risk. Makes the API client untestable in isolation. Violates single responsibility.

**Instead:** API client returns typed responses. The stage (caller) is responsible for persisting mappings and updating state. API client is a pure HTTP abstraction.

### Anti-Pattern 3: Storing PlentyONE IDs In-Memory Only

**What:** Keeping the local-to-PlentyONE ID mappings in a map during pipeline execution without persisting to MySQL after each operation.

**Why bad:** If the process crashes mid-pipeline, all ID mappings from completed stages are lost. You have orphaned entities in PlentyONE with no record of their IDs. Recovery is nearly impossible at scale.

**Instead:** Persist every mapping to MySQL immediately after each successful API call. This enables resume-from-failure and provides the audit trail.

### Anti-Pattern 4: Shared Mutable Config Object

**What:** A global config struct that various packages read and write.

**Why bad:** Race conditions. Hard to test. Unclear who owns configuration.

**Instead:** Load config once at startup in `app`. Pass relevant config values to each component via constructors. Components receive only the config they need.

### Anti-Pattern 5: Tight Coupling CLI and Server

**What:** CLI directly imports dashboard handlers or the server imports CLI command definitions.

**Why bad:** Both entry points should be thin wrappers. If they share code, it should be through the application layer, not through each other.

**Instead:** Both `cmd/server/` and `cmd/cli/` import `internal/app` which wires up shared services. They never import each other.

## Key Database Schema Concepts

The MySQL schema needs to support four concerns:

### 1. Entity Mappings (the core purpose)

```sql
CREATE TABLE entity_mappings (
    id         BIGINT AUTO_INCREMENT PRIMARY KEY,
    run_id     BIGINT NOT NULL,
    local_id   BIGINT NOT NULL,
    plenty_id  BIGINT NOT NULL,
    entity_type VARCHAR(50) NOT NULL,  -- 'category', 'attribute', 'product', 'variation', etc.
    stage      VARCHAR(50) NOT NULL,   -- which pipeline stage created this
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY idx_local_entity (local_id, entity_type),
    INDEX idx_run (run_id),
    INDEX idx_plenty (plenty_id, entity_type)
);
```

### 2. Pipeline State (for resume support)

```sql
CREATE TABLE pipeline_runs (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    job_id      BIGINT NOT NULL,
    status      VARCHAR(20) NOT NULL,  -- pending, running, paused, completed, failed
    current_stage VARCHAR(50),
    started_at  TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE stage_states (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    run_id      BIGINT NOT NULL,
    stage_name  VARCHAR(50) NOT NULL,
    status      VARCHAR(20) NOT NULL,
    processed   INT DEFAULT 0,
    total       INT DEFAULT 0,
    error_detail TEXT,
    started_at  TIMESTAMP,
    completed_at TIMESTAMP,
    UNIQUE KEY idx_run_stage (run_id, stage_name)
);
```

### 3. Generated Product Data (pipeline input)

```sql
CREATE TABLE generated_products (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    job_id      BIGINT NOT NULL,
    product_type VARCHAR(100),
    base_data   JSON NOT NULL,     -- structured product data
    status      VARCHAR(20) NOT NULL DEFAULT 'generated',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE generated_texts (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    product_id  BIGINT NOT NULL,
    field       VARCHAR(50) NOT NULL,  -- 'name', 'description', 'short_description'
    lang        VARCHAR(5) NOT NULL,   -- 'en', 'de', 'es', 'fr', 'it'
    content     TEXT NOT NULL,
    UNIQUE KEY idx_product_field_lang (product_id, field, lang)
);
```

### 4. Job Tracking

```sql
CREATE TABLE jobs (
    id          BIGINT AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(255),
    config      JSON NOT NULL,     -- job configuration snapshot
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

## Scalability Considerations

| Concern | At 100 products | At 1K products | At 10K products |
|---------|----------------|----------------|-----------------|
| API calls | ~600 calls, single-threaded fine | ~6K calls, need rate limiting, ~1hr | ~60K calls, need batch + rate limit, ~10hrs |
| MySQL load | Negligible | Moderate (6K mapping inserts) | Heavy (60K inserts), use batch inserts |
| Memory | All in memory fine | Fine with streaming | Must process in chunks, not load all at once |
| Error handling | Manual review feasible | Need filtering/search in dashboard | Need bulk operations (skip all similar errors) |
| Pipeline resume | Rarely needed | Occasionally needed | Essential -- will fail partway through |
| Rate limiting | Probably not hit | Will hit PlentyONE rate limits | Must be carefully tuned, consider parallel stages for independent entities |

**Key scaling decisions:**
- Process entities within a stage in configurable batch sizes (e.g., 50 at a time)
- Use `database/sql`'s connection pooling (built-in) with tuned `MaxOpenConns`
- Rate limiter must be configurable -- PlentyONE rate limits vary by endpoint and plan
- At 10K+, consider concurrent processing WITHIN a stage (e.g., create 10 categories concurrently) but NEVER across stages (strict ordering)

## Suggested Build Order

Dependencies between components determine what gets built first. This ordering ensures each component can be tested independently as it is built.

```
Phase 1: Foundation
  domain/     -> Shared types, no dependencies. Build first.
  storage/    -> MySQL schema, migrations, repos. Depends on domain.
  app/config  -> Configuration loading. No service deps.

Phase 2: External Clients
  plentyapi/  -> API client with OAuth + rate limiting. Depends on domain.
                 Can be tested against PlentyONE sandbox independently.
  datasource/ -> AI providers, public DB clients, image sources.
                 Each provider can be tested independently.

Phase 3: Core Logic
  pipeline/   -> Orchestrator + stages. Depends on storage, plentyapi, datasource.
                 This is the brain. Build after Phase 2 so it has real clients to use.
  job/        -> Job management. Depends on storage, pipeline.

Phase 4: Entry Points
  cmd/cli/    -> CLI commands. Depends on app, job, pipeline.
  dashboard/  -> Web handlers, templates. Depends on app, job, pipeline, storage.
  cmd/server/ -> HTTP server wiring. Depends on dashboard.
```

**Why this order:**
1. `domain` first because everything imports it and it imports nothing.
2. `storage` second because pipeline stages need to persist mappings -- can't test stages without storage.
3. API clients third because pipeline stages call them -- can't test stages without clients.
4. Pipeline fourth because it IS the product. Once domain, storage, and clients exist, the pipeline can be built and tested end-to-end.
5. Entry points last because they are thin wrappers. CLI and dashboard consume services -- they don't define behavior.

**Critical dependency:** The pipeline stages cannot be built until both `storage` (for mappings) and `plentyapi` (for API calls) exist. Building these in parallel accelerates the timeline.

## Technology Choices Implied by Architecture

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| HTTP router | `net/http` (stdlib) with a lightweight mux | Dashboard is simple CRUD + SSE. No need for a framework. `chi` or `http.ServeMux` (Go 1.22+) both work. |
| CLI framework | `cobra` | De facto standard for Go CLIs. Subcommands map naturally to job operations. |
| Database driver | `go-sql-driver/mysql` | Standard MySQL driver for Go. Use with `database/sql`. |
| Migrations | `golang-migrate/migrate` | File-based SQL migrations. Simple, reliable. |
| Config | `koanf` or `viper` | YAML config file + environment variable overrides. |
| Rate limiting | `golang.org/x/time/rate` | Standard library extension. Token bucket algorithm. |
| HTML templates | `html/template` (stdlib) | Dashboard does not need a SPA. Server-rendered HTML with HTMX for interactivity. |
| Frontend interactivity | HTMX + minimal JS | Server-driven UI. No React/Vue build step. Perfect for Go dashboard pattern. |
| Logging | `log/slog` (stdlib, Go 1.21+) | Structured logging built into stdlib. No external dependency needed. |
| Testing | `testing` (stdlib) + `testcontainers-go` for MySQL | Integration tests with real MySQL in Docker. |

## Sources

- Go project layout: Based on patterns from well-known Go projects (Kubernetes, Docker CLI, HashiCorp tools). The `cmd/` + `internal/` pattern is documented in Go blog posts and widely adopted. Confidence: HIGH.
- Pipeline/stage pattern: Standard state machine pattern adapted for Go. Used in CI/CD systems (Drone, Woodpecker), ETL tools, and data pipelines. Confidence: HIGH.
- Repository pattern for Go + SQL: Widely used in Go services. Avoids ORM complexity while keeping SQL out of business logic. Confidence: HIGH.
- OAuth transport pattern: Based on `golang.org/x/oauth2` package's approach of wrapping `http.RoundTripper`. Well-established Go idiom. Confidence: HIGH.
- HTMX for Go dashboards: Growing pattern in Go community as alternative to SPA dashboards. Simple, works with Go templates. Confidence: MEDIUM (verify HTMX version compatibility).
- PlentyONE API specifics (rate limits, OAuth flow, entity relationships): Based on project requirements. Actual API behavior should be verified against PlentyONE API docs during implementation. Confidence: MEDIUM.
