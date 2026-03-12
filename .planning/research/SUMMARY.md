# Project Research Summary

**Project:** PlentyOne Product Generator
**Domain:** E-commerce product data generation with AI and PlentyONE API integration
**Researched:** 2026-03-12
**Confidence:** MEDIUM

## Executive Summary

PlentyOne is a Go-based tool that generates realistic e-commerce product data using AI (text, images) and public databases, then pushes that data into a PlentyONE shop via its REST API through a strict 6-stage pipeline (categories, attributes, products, variations, images/properties, multilingual text). This is fundamentally a **data pipeline engineering problem** wrapped in an AI integration layer. Experts build this type of system as a state-machine-driven pipeline with per-entity tracking, idempotent stages, and pause-and-resume semantics -- not as a simple script that fires API calls sequentially. The recommended stack is pure Go with chi router, cobra/viper for CLI, sqlc for type-safe MySQL access, and templ + HTMX for a server-rendered dashboard. The entire application compiles to a single binary with MySQL as the only external dependency.

The recommended approach is to build from the inside out: domain types first, then the MySQL schema (designed for failure states and resume from day one), then the PlentyONE API client with OAuth and rate limiting, then the pipeline orchestrator, then individual stages, and finally the user-facing layers (CLI, then dashboard). AI provider integration should use a clean interface pattern, starting with OpenAI only. The dashboard is valuable but not MVP-critical -- the CLI is sufficient for initial operation. A custom pipeline engine (~500-800 lines) is strongly preferred over generic job queue frameworks (asynq, river, machinery) because the pipeline has domain-specific semantics (strict ordering, per-item state, pause-and-flag) that generic queues fight rather than support.

The top risks are: (1) orphaned entities in PlentyONE from partial pipeline failures with no rollback API, requiring a cleanup utility and meticulous state tracking; (2) PlentyONE rate limiting being stricter than expected, requiring a centralized rate limiter tested at scale early; (3) OAuth token refresh race conditions under concurrent API calls; and (4) AI-generated text failing PlentyONE field validation, requiring a sanitization/validation layer between generation and submission. All four of these risks are addressable with upfront architectural decisions -- the danger is deferring them.

## Key Findings

### Recommended Stack

The stack centers on Go's strong standard library, augmented with a small set of well-established community packages. No JavaScript framework or Redis/message queue dependency is needed. Both sqlc (SQL-to-Go codegen) and templ (HTML-to-Go codegen) require a build-time code generation step, managed via Taskfile.

**Core technologies:**
- **Go 1.22+** with **chi/v5** router: Lightweight HTTP routing following stdlib conventions, with method-based routing, middleware chaining, and route groups
- **cobra + viper**: CLI framework and configuration management -- the de facto Go standard for both
- **sqlc + golang-migrate**: SQL-first database access with compile-time type safety, plus file-based schema migrations. Strongly preferred over GORM or sqlx for this schema-heavy project
- **Custom pipeline engine** with errgroup: Purpose-built state machine for the 6-stage pipeline. Generic job queues rejected (asynq needs Redis, river needs Postgres, neither matches the ordered state machine semantics)
- **templ + HTMX + Tailwind CSS**: Server-rendered dashboard with dynamic updates via HTMX. No SPA, no Node.js, no frontend build complexity
- **slog** (stdlib): Structured logging with correlation IDs per batch/product
- **golang.org/x/oauth2 + golang.org/x/time/rate**: OAuth token management and rate limiting for all PlentyONE API calls

**Version verification needed before implementation:** templ (may have hit v1.0), anthropic-sdk-go (was very new at training cutoff), Tailwind CSS (v3.4 vs v4 transition). All other packages are stable and versions are HIGH confidence.

### Expected Features

**Must have (table stakes):**
- AI-powered product text generation with configurable providers (start with OpenAI)
- Multilingual text generation (EN, DE, ES, FR, IT) -- generate natively per language, not translate
- Configurable product niche/type for coherent data across categories
- Image sourcing from free stock APIs (start with Pexels, best rate limits)
- Strict 6-stage pipeline: categories -> attributes -> products -> variations -> images -> text
- Entity ID mapping (local UUID <-> PlentyONE integer ID) with per-item status tracking
- OAuth token management with proactive refresh before expiry
- Rate limiting with exponential backoff and Retry-After header parsing
- Pause-and-flag error handling (flag failed items, continue with others)
- CLI with core commands: generate, push, status, retry, config
- Structured logging with correlation IDs

**Should have (differentiators):**
- Niche-aware generation templates (food, electronics, fashion) with niche-specific fields
- Dry-run / simulation mode (validate entire batch without calling PlentyONE API)
- Data quality scoring (completeness, realism, SEO quality per product)
- Incremental / resumable pipeline (one command to "continue where we left off")
- Existing catalog detection (check PlentyONE for existing entities before creating)
- Web dashboard with pipeline status, data preview, configuration UI, mapping overview
- Product data export (CSV/JSON)

**Defer to v2+:**
- AI image generation (stock photos are faster and more realistic initially)
- Public database integration (Open Food Facts, Wikidata) -- pure AI generation works for v1
- Two-way sync, product updates/editing in PlentyONE
- Multi-shop support, batch scheduling, SEO optimization
- Seed data augmentation (few-shot from real products)
- Multi-platform support (Shopify, WooCommerce)

### Architecture Approach

A layered service architecture with three entry points (CLI, HTTP server for dashboard, potentially combined into one binary) sharing a common application core. Dependencies flow inward: domain types have zero imports, storage and API clients depend only on domain, the pipeline orchestrator depends on storage + clients, and entry points are thin wrappers. The project uses `internal/` for everything (this is a service, not a library) with `cmd/server/` and `cmd/cli/` as separate binaries.

**Major components:**
1. **domain/** -- Shared types (Product, Variation, Category, Mapping, Job, Pipeline state enums). Zero dependencies. Everything imports this.
2. **storage/** -- MySQL data access via sqlc-generated code. Repository pattern. Handles entity mappings, pipeline state, generated product data, job tracking.
3. **plentyapi/** -- PlentyONE REST API client. OAuth transport with mutex-guarded token refresh, centralized rate limiter, entity-type-specific methods behind common patterns.
4. **datasource/** -- AI provider interface (TextGenerator, ImageSource) with OpenAI/Anthropic implementations, plus public database and stock image clients.
5. **pipeline/** -- The heart of the system. Stage interface with state machine orchestrator. Manages strict ordering, per-item state, pause-and-flag, resume-from-failure. ~500-800 lines of purpose-built code.
6. **dashboard/** -- HTTP handlers + templ templates + SSE for live pipeline status updates. Server-rendered, no SPA.
7. **app/** -- Wiring layer. Loads config, creates DB connections, instantiates all services, dependency-injects into components.

### Critical Pitfalls

1. **Orphaned entities from partial pipeline failure** -- PlentyONE has no transactional rollback. Persist every mapping to MySQL immediately after each API call. Build a cleanup/rollback utility. Track per-entity status (pending/created/linked/complete/failed/orphaned).
2. **PlentyONE rate limiting stricter than expected** -- Centralized HTTP client with token bucket rate limiter. Parse Retry-After headers. Exponential backoff with jitter. Test at realistic scale (500+ products) early in development, not after "everything works."
3. **OAuth token refresh race condition** -- Use `oauth2.ReuseTokenSource` or a mutex-guarded refresh pattern. Only one goroutine refreshes while others block and wait. Persist tokens to MySQL for restart survival.
4. **AI-generated data fails PlentyONE validation** -- Build a validation/sanitization layer between AI output and PlentyONE submission. Enforce field length limits, strip HTML, validate language. Store raw AI output separately from sanitized output.
5. **MySQL schema that cannot handle pipeline resume** -- Design the schema around pipeline runs and jobs, not just entity mappings. Every entity mapping needs job_id, stage, status, error_message. Build "resume from failure" as a first-class operation from day one.

## Implications for Roadmap

Based on combined research, the project naturally divides into 6 phases driven by dependency ordering and risk mitigation. The critical path is: schema -> API client -> pipeline engine -> pipeline stages -> CLI -> dashboard.

### Phase 1: Foundation (Domain + Schema + Configuration)

**Rationale:** Every other component depends on domain types and the MySQL schema. The schema must be designed for failure states and resume semantics from the start (Pitfall 5). Getting this wrong forces a rewrite.
**Delivers:** Go module initialized, domain types, MySQL schema with migrations (entity_mappings, pipeline_runs, stage_states, jobs, generated_products, generated_texts), sqlc-generated data access layer, configuration loading via viper, project structure with cmd/ and internal/.
**Addresses:** Core data model (categories, attributes, products, variations), entity ID mapping, price/SKU generation data structures.
**Avoids:** Pitfall 5 (schema that cannot handle resume), Pitfall 1 (orphaned entities -- by including status fields from day one).

### Phase 2: PlentyONE API Client (OAuth + Rate Limiting + Entity Methods)

**Rationale:** The pipeline cannot be built or tested without a working API client. OAuth and rate limiting must be correct before any pipeline work begins (Pitfalls 2, 3). Entity-type-specific clients are needed because the PlentyONE API is inconsistent across entity types (Pitfall 9).
**Delivers:** HTTP client wrapper with OAuth transport (mutex-guarded token refresh), centralized rate limiter, Retry-After handling, exponential backoff, entity-specific methods for categories, attributes, products, variations, images, and text.
**Uses:** net/http, golang.org/x/oauth2, golang.org/x/time/rate, go-sql-driver/mysql (for token persistence).
**Avoids:** Pitfall 2 (rate limiting underestimation), Pitfall 3 (OAuth race condition), Pitfall 9 (API inconsistencies).

### Phase 3: AI Integration + Data Generation

**Rationale:** AI-generated data is the primary input to the pipeline. The provider interface, validation layer, and data generation logic must exist before the pipeline can process real data. Building the mock provider first controls AI API costs (Pitfall 11).
**Delivers:** TextGenerator and ImageSource interfaces, OpenAI implementation, mock provider for development, product data generation logic, validation/sanitization layer between AI output and PlentyONE field constraints, image download and local storage pipeline (Pitfall 6), multilingual text generation with per-language prompts.
**Addresses:** AI-powered text generation, multilingual generation, configurable niche/type, image sourcing from stock APIs.
**Avoids:** Pitfall 4 (AI data fails validation), Pitfall 6 (image URL expiration), Pitfall 11 (cost explosion), Pitfall 12 (multilingual quality).

### Phase 4: Pipeline Orchestrator + Stages

**Rationale:** This is the heart of the system. It requires working storage (Phase 1), API client (Phase 2), and data generation (Phase 3). Build the orchestrator (state machine, stage interface, resume logic) first, then implement each stage in PlentyONE's required order.
**Delivers:** Pipeline orchestrator with Stage interface, state machine transitions, pause-and-flag error handling, resume-from-failure, all 6 pipeline stages (categories, attributes, products, variations, images/properties, text), entity mapping persistence after every API call, cleanup/rollback utility for orphaned entities, bounded concurrency via errgroup worker pool.
**Addresses:** 6-stage pipeline execution, batch processing, error handling, retry mechanism, incremental/resumable pipeline.
**Avoids:** Pitfall 1 (orphaned entities), Pitfall 8 (goroutine leaks).

### Phase 5: CLI

**Rationale:** The CLI is the minimum viable user interface. Once the pipeline works (Phase 4), the CLI wraps it with human-friendly commands. This is low-risk, low-complexity work using battle-tested cobra patterns.
**Delivers:** cobra-based CLI with subcommands: generate (trigger data generation), push (run pipeline), status (show job/pipeline progress), retry (resume failed jobs), config (manage settings). Structured log output. Dry-run mode.
**Addresses:** CLI for job management, configuration management, logging output.

### Phase 6: Web Dashboard

**Rationale:** The dashboard is a valuable enhancement but not MVP-critical. Building it last means the underlying services are stable and well-tested. The dashboard is a read-heavy consumer of existing services, not a producer of new logic.
**Delivers:** templ + HTMX server-rendered dashboard with pipeline status view (SSE for live updates), data preview (generated products table/cards), configuration UI (form-based with validation), mapping overview (entity mappings with filtering). In-memory status summary to avoid DB polling overhead (Pitfall 10).
**Addresses:** Pipeline status view, data preview, configuration UI, mapping overview.
**Avoids:** Pitfall 10 (dashboard polling overload).

### Phase Ordering Rationale

- **Phases 1-2 are non-negotiable first steps.** Every other component depends on domain types, schema, and the API client. Building them first also forces early confrontation with the most critical pitfalls (schema design, rate limiting, OAuth).
- **Phase 3 before Phase 4** because the pipeline needs real data to process. Building AI integration before the pipeline also allows the validation layer to be tested independently before it is wired into the pipeline.
- **Phase 4 is the architectural centerpiece** and carries the highest risk. By the time it begins, all dependencies are in place and tested.
- **Phase 5 before Phase 6** because the CLI is both simpler and sufficient for initial operation. It also serves as the first real integration test of the full system.
- **Phase 6 last** because it is additive UX, not core functionality. It can be built (or deferred) without affecting the pipeline.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 2 (PlentyONE API Client):** PlentyONE-specific rate limits, OAuth flow, endpoint behaviors, and sandbox vs. production differences need verification against current API documentation. Training data confidence is MEDIUM for PlentyONE specifics.
- **Phase 3 (AI Integration):** Anthropic Go SDK maturity needs verification (was very new at training cutoff). Prompt engineering for multilingual quality is iterative and hard to research ahead of time.
- **Phase 4 (Pipeline Stages):** Each of the 6 pipeline stages will encounter PlentyONE API quirks that are not fully documented. Plan for discovery and iteration, especially around variation creation (most complex entity type).

Phases with standard patterns (skip deep research):
- **Phase 1 (Foundation):** Go project structure, MySQL schema design, sqlc codegen, and cobra/viper setup are all extremely well-documented with established patterns. HIGH confidence.
- **Phase 5 (CLI):** Cobra CLI patterns are the most documented Go pattern. No research needed.
- **Phase 6 (Dashboard):** templ + HTMX + SSE is a well-established Go dashboard pattern. Standard implementation.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Core Go ecosystem (chi, cobra, viper, sqlc, slog, errgroup) is stable and well-established. Only templ version and Anthropic SDK need verification. |
| Features | MEDIUM | Feature categorization is solid based on e-commerce domain patterns. Complexity estimates and specific PlentyONE field constraints need validation against live API. |
| Architecture | HIGH | Layered service architecture, pipeline state machine, repository pattern, and provider interfaces are all well-proven Go patterns. Project layout follows established conventions. |
| Pitfalls | MEDIUM-HIGH | Go concurrency pitfalls and pipeline state management are HIGH confidence. PlentyONE-specific pitfalls (rate limits, API inconsistencies) are MEDIUM -- based on training data for plentymarkets, needs verification against current PlentyONE docs. |

**Overall confidence:** MEDIUM -- the Go ecosystem recommendations and architectural patterns are rock-solid, but PlentyONE API specifics and AI provider SDK maturity are the weak spots. Both are addressable with targeted verification during Phase 2 and Phase 3 planning.

### Gaps to Address

- **PlentyONE API rate limits:** Exact per-endpoint limits, cooldown behavior, and sandbox vs. production differences need verification against current documentation before Phase 2 implementation.
- **PlentyONE entity creation specifics:** Request/response formats, required fields, and validation rules per entity type need discovery during Phase 2. Plan for an API exploration spike.
- **Anthropic Go SDK status:** Verify package path, API coverage, and maturity before committing to SDK vs. thin HTTP wrapper approach. LOW confidence from research.
- **templ version and API stability:** May have hit v1.0 since training cutoff. Verify current version and any breaking changes before Phase 6.
- **AI prompt engineering for multilingual quality:** Cannot be fully researched ahead of time. Plan for iterative prompt development during Phase 3 with quality evaluation checkpoints.
- **PlentyONE image upload requirements:** Exact format, size, and compression requirements need verification. Determines whether an image processing sub-pipeline is needed in Phase 3 or can be deferred.

## Sources

### Primary (HIGH confidence)
- Go standard library and golang.org/x packages (oauth2, time/rate, sync/errgroup) -- stable, well-documented
- Go community ecosystem packages (chi, cobra, viper, sqlc, golang-migrate, templ) -- established, widely adopted
- Go project layout conventions (cmd/, internal/ pattern) -- documented in Go blog, used by Kubernetes/Docker/HashiCorp
- Pipeline/state machine patterns -- standard in CI/CD systems, ETL tools, data pipelines
- Open Food Facts license (ODbL) and Wikidata license (CC0) -- factual, well-established

### Secondary (MEDIUM confidence)
- PlentyONE/plentymarkets REST API patterns (rate limits, OAuth flow, entity relationships) -- based on training data for plentymarkets, verify against current PlentyONE docs
- OpenAI Go SDK (sashabaranov/go-openai) -- mature but tracks fast-moving API
- Stock photo API details (Unsplash, Pexels, Pixabay rate limits and licensing) -- verify current terms
- E-commerce PIM/product data patterns -- based on domain knowledge of Akeneo, Shopify, WooCommerce import tools
- HTMX + templ dashboard pattern -- growing adoption in Go community, verify version compatibility

### Tertiary (LOW confidence)
- Anthropic Go SDK (anthropic-sdk-go) -- was very new at training cutoff, package path and API coverage need verification
- Exact version numbers for all packages -- verify before go mod init
- Tailwind CSS v3.4 vs v4 transition status -- verify current recommended version

---
*Research completed: 2026-03-12*
*Ready for roadmap: yes*
