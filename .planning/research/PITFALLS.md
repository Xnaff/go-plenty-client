# Domain Pitfalls

**Domain:** E-commerce product data generation with PlentyONE API integration (Go)
**Researched:** 2026-03-12
**Overall confidence:** MEDIUM (WebSearch/WebFetch unavailable; based on training data for plentymarkets API specifics, HIGH confidence for Go patterns and pipeline orchestration)

---

## Critical Pitfalls

Mistakes that cause rewrites, data corruption, or major project delays.

---

### Pitfall 1: Orphaned Entities from Partial Pipeline Failure

**What goes wrong:** The 6-stage pipeline creates categories, attributes, products, variations, properties, images, and multilingual text in strict order. If stage 4 (create variations) fails after stage 3 (create parent product) succeeds, you have a parent product in PlentyONE with no variations. There is no transactional rollback across API calls. Multiply this by thousands of products and you get a PlentyONE instance full of half-created garbage entities that are hard to identify and harder to clean up.

**Why it happens:** Developers treat the pipeline as a happy-path sequence and bolt on error handling later. The PlentyONE REST API does not offer multi-step transactions. Each POST is final. There is no "undo" endpoint for most entity types. Rate limit hits, network timeouts, or validation errors at any stage leave prior stages committed.

**Consequences:**
- PlentyONE instance polluted with orphaned categories, empty products, variations without images/text
- Manual cleanup requires identifying which entities are orphans (non-trivial at scale)
- Re-running the pipeline creates duplicates unless idempotency is built in
- Customer loses trust in the tool if their PlentyONE shop is littered with junk data

**Prevention:**
1. Design the MySQL mapping table as a state machine from day one. Every entity gets a status: `pending`, `created`, `linked`, `complete`, `failed`, `orphaned`.
2. Implement a "watermark" per product: track exactly which stage completed successfully. On failure, mark the product at its last successful stage and flag it for review.
3. Build a cleanup/rollback utility that can DELETE orphaned entities from PlentyONE using the mapping table. This is not optional -- it is a core feature.
4. Never auto-retry a failed stage without human review (the "pause-and-flag" requirement is correct here).
5. Before creating any entity, check the mapping table: if a local ID already has a PlentyONE ID for this stage, skip creation (idempotency).

**Detection (warning signs):**
- Pipeline code that catches errors and just logs them without updating state
- No `DELETE` API calls anywhere in the codebase
- Mapping table has only `local_id` and `plenty_id` columns with no status field
- No integration tests that simulate mid-pipeline failure

**Phase mapping:** Must be addressed in the core pipeline/infrastructure phase. Retrofitting state tracking onto an existing pipeline is a rewrite.

**Confidence:** HIGH -- this is inherent to any multi-step API integration without transactions.

---

### Pitfall 2: PlentyONE Rate Limiting is Stricter Than Expected

**What goes wrong:** The plentymarkets/PlentyONE REST API enforces short-term rate limits (historically around 60 calls per minute for certain endpoints, with a "leaky bucket" or token bucket model). Developers calculate "thousands of products times 7 stages = ~7000 API calls" and assume a simple sleep-between-calls approach. In reality, different endpoints may have different limits, rate limit responses can be inconsistent, and aggressive retry loops can trigger longer cooldowns or temporary bans.

**Why it happens:**
- Rate limit documentation may not cover all edge cases or endpoint-specific limits
- Developers test with 10 products (no rate limit hit) and discover the problem at 500+
- Naive retry logic (immediate retry on 429) makes the problem worse
- Token refresh calls also count against rate limits

**Consequences:**
- Pipeline stalls for minutes at a time, making a 1000-product batch take hours instead of the expected ~30 minutes
- Aggressive retry causes escalating cooldowns
- OAuth token refresh fails during a rate limit window, causing auth errors that cascade
- User perceives the tool as broken when it is "just" throttled

**Prevention:**
1. Build a centralized HTTP client with a token bucket rate limiter (Go's `golang.org/x/time/rate` package). All PlentyONE API calls must go through this single client.
2. Parse `Retry-After` headers from 429 responses and respect them exactly. Do not guess.
3. Implement exponential backoff with jitter on rate limit errors, capped at a reasonable maximum (e.g., 60 seconds).
4. Separate rate limit budgets for "critical" calls (token refresh) vs. "bulk" calls (product creation). Token refresh should have priority.
5. Add a throughput dashboard metric showing calls/minute and remaining quota. This makes rate limiting visible to the user.
6. Test with realistic batch sizes (500+ products) early in development. Do not wait until "everything works" to test at scale.

**Detection (warning signs):**
- HTTP client that does not handle 429 status codes explicitly
- No rate limiter middleware in the API client
- All API calls share a single retry loop with no backoff
- Integration tests only use 1-5 products

**Phase mapping:** Must be built into the API client layer from the start. The HTTP client and rate limiter are foundational -- everything else depends on them.

**Confidence:** MEDIUM -- rate limit specifics may have changed in PlentyONE vs. legacy plentymarkets. Verify exact limits against current API docs during implementation.

---

### Pitfall 3: OAuth Token Refresh Race Condition Under Concurrency

**What goes wrong:** When processing products in parallel (even modest concurrency of 3-5 goroutines), multiple goroutines discover the OAuth token is expired at roughly the same time. Each one independently tries to refresh the token. The first refresh succeeds and invalidates the old refresh token. Subsequent refresh attempts fail because they are using the now-invalidated refresh token. This cascades into auth failures across all goroutines.

**Why it happens:**
- Go makes concurrency easy (`go func()`) so developers parallelize API calls early
- OAuth2 token refresh is a stateful operation -- the refresh token is single-use in many implementations
- plentymarkets OAuth historically invalidates the previous refresh token on use
- Standard `golang.org/x/oauth2` package handles this correctly IF configured properly, but custom OAuth implementations often miss this

**Consequences:**
- Intermittent 401 errors that are impossible to reproduce with sequential testing
- Pipeline appears to work fine with low concurrency, breaks under load
- Difficult to debug because the race condition is timing-dependent

**Prevention:**
1. Use a single token source with mutex-protected refresh. Go's `golang.org/x/oauth2` package provides `oauth2.ReuseTokenSource` which handles this. Use it.
2. If implementing custom OAuth: use a `sync.Once`-like pattern where only one goroutine refreshes while others block and wait for the result.
3. Store the current token in a thread-safe container (e.g., `atomic.Value` or mutex-protected struct) that all goroutines read from.
4. Persist tokens to MySQL so that service restarts do not require re-authentication.
5. Test with concurrent goroutines AND an expired token to verify the refresh path.

**Detection (warning signs):**
- OAuth token stored in a plain variable accessed by multiple goroutines
- Token refresh logic that does not use a mutex or sync primitive
- No test that forces a token refresh under concurrent load
- Intermittent 401 errors in logs that "go away on retry"

**Phase mapping:** API client foundation phase. The token management must be correct before any pipeline work begins.

**Confidence:** HIGH -- this is a well-documented Go concurrency pitfall with OAuth2.

---

### Pitfall 4: AI-Generated Product Data Fails PlentyONE Validation

**What goes wrong:** AI providers (OpenAI, Anthropic, etc.) generate product text that looks reasonable to humans but fails PlentyONE API validation. Examples: descriptions exceeding character limits, property values not matching allowed enum values, category names with forbidden characters, image URLs that are not directly accessible (AI image providers return temporary URLs that expire), HTML in fields that expect plain text, or text in the wrong language.

**Why it happens:**
- AI output is non-deterministic. Even with strong prompts, LLMs produce unexpected formats.
- PlentyONE has strict field-level validation that is not fully documented in a machine-readable schema.
- Developers test with a few AI outputs, declare it "working", and hit edge cases at scale.
- Different AI providers format output differently (Markdown vs. plain text, different quote styles, etc.)

**Consequences:**
- Stage 6 (multilingual text) or stage 5 (properties) fails for 20% of products, creating orphaned entities (compounds Pitfall 1)
- Debugging requires correlating AI output with PlentyONE validation errors -- tedious at scale
- Re-generating AI output is expensive (API costs) and may not fix the issue without prompt changes

**Prevention:**
1. Build a validation layer between AI generation and PlentyONE submission. Validate ALL fields against known PlentyONE constraints BEFORE making any API calls.
2. Define strict output schemas for AI prompts. Use JSON mode / structured output where available. Parse and validate the structured output.
3. Implement field-level sanitization: trim to max length, strip HTML, normalize Unicode, enforce character whitelist.
4. For multilingual text: validate language detection on AI output. A "French" description that is actually English wastes a pipeline slot.
5. Cache and re-use validated AI output. Do not re-generate on pipeline retry -- only re-generate on explicit user request.
6. Store raw AI output separately from sanitized output. This enables debugging without re-calling the AI.

**Detection (warning signs):**
- AI output piped directly to PlentyONE API without intermediate validation
- No character limit enforcement on generated text
- No structured output / JSON mode in AI prompts
- Pipeline failures clustered in later stages (text, properties) rather than early stages

**Phase mapping:** The validation/sanitization layer should be built alongside AI integration, before connecting to the PlentyONE pipeline. Test AI output against validation rules independently.

**Confidence:** HIGH -- AI output validation is a universal challenge in LLM-to-API pipelines.

---

### Pitfall 5: MySQL Mapping Table Schema That Cannot Handle Pipeline Resume

**What goes wrong:** The mapping table is designed as a simple two-column lookup (local_id, plenty_id) per entity type. When a pipeline run fails at stage 4 for product #347 out of 1000, there is no way to resume from that exact point. The pipeline either re-runs everything (creating duplicates) or requires manual identification of where it stopped.

**Why it happens:**
- Developers design the database for the happy path (complete products) not the failure path (partial products)
- "We will add resume logic later" -- except the schema does not support it
- No concept of a "pipeline run" or "batch job" in the schema

**Consequences:**
- Duplicate entities in PlentyONE after re-running a failed batch
- Hours wasted manually figuring out where a batch stopped
- Users lose confidence and stop running large batches

**Prevention:**
1. Design the schema around pipeline runs/jobs, not just entities. Every job has an ID, status, and progress watermark.
2. Each entity-to-PlentyONE mapping must include: `job_id`, `stage`, `status` (pending/created/failed), `plenty_id` (nullable until created), `error_message`, `created_at`, `updated_at`.
3. Build "resume from failure" as a first-class operation: query for `job_id=X AND status=failed`, then re-process only those entities from their failed stage.
4. Add a `plenty_external_key` or natural key column for deduplication. Before creating, query PlentyONE to check if the entity already exists (by name, SKU, etc.).
5. Use database transactions for local state updates even though you cannot use them for API calls.

**Detection (warning signs):**
- Mapping tables with only (local_id, plenty_id) and no status column
- No `jobs` or `pipeline_runs` table
- Resume logic that starts from stage 1 regardless of prior progress
- No deduplication check before API creation calls

**Phase mapping:** Database schema design phase -- must be right before any pipeline code is written. Schema migration later is possible but painful.

**Confidence:** HIGH -- standard data pipeline state management pattern.

---

## Moderate Pitfalls

---

### Pitfall 6: Image Upload Timing and URL Expiration

**What goes wrong:** AI image generation services (DALL-E, Midjourney API, Stable Diffusion) and stock photo APIs (Unsplash, Pexels, Pixabay) return URLs with different lifetimes. AI-generated image URLs often expire within minutes to hours. If images are generated in an early batch step but uploaded to PlentyONE hours later (after processing thousands of other products), the URLs have expired. PlentyONE rejects the upload or silently creates a broken image reference.

**Why it happens:**
- Pipeline stages are sequential per product but batched across products. Image generation and image upload may be separated by significant time.
- Developers test with small batches where timing is not an issue.
- Different image sources have different expiration policies, and this is not always documented.

**Prevention:**
1. Download and store images locally (or in object storage) immediately upon generation. Never pass a temporary URL to PlentyONE.
2. Store images as binary blobs or local file paths, not URLs, in the mapping database.
3. Implement image download as part of the generation stage, not the upload stage.
4. Validate image accessibility before upload: HTTP HEAD check to confirm the image is still downloadable.
5. Set a maximum age for cached images and re-download if expired.

**Detection (warning signs):**
- Image URLs stored in the database instead of local file paths
- No image download step between generation and PlentyONE upload
- Broken images in PlentyONE after large batch runs but not small ones

**Phase mapping:** Image handling infrastructure, built during or before the AI integration phase.

**Confidence:** HIGH -- well-known issue with AI image APIs.

---

### Pitfall 7: Public Database Licensing Violations

**What goes wrong:** Open Food Facts uses the Open Database License (ODbL), which requires attribution and share-alike for derivative databases. Wikidata uses CC0 (public domain). Developers treat them identically, mix data without tracking provenance, and accidentally create licensing obligations they are unaware of. Worse, some "public" APIs have terms of service that restrict commercial use or bulk downloading even when the data license appears open.

**Why it happens:**
- "Open data" is assumed to mean "do whatever you want."
- ODbL's share-alike clause is subtle: if you create a new database that is substantially derived from an ODbL database, the new database must also be ODbL.
- Terms of Service and data licenses are different things. An API's ToS may restrict bulk access even if the underlying data is openly licensed.
- Stock image APIs (Unsplash, Pexels) have specific license terms that prohibit certain uses (e.g., using images to create competing services).

**Prevention:**
1. Track data provenance per field: which source contributed which data to each product.
2. Store source attribution metadata alongside product data.
3. Review each data source's license AND terms of service separately. Document them in a LICENSES.md.
4. For ODbL data (Open Food Facts): ensure attribution is displayed and understand share-alike implications.
5. For stock images: comply with each API's specific attribution requirements (Unsplash requires photographer credit, Pexels has different rules, Pixabay recently changed their license).
6. Implement rate limiting for public database APIs to stay within their acceptable use policies.
7. Consider caching/mirroring public data locally rather than hitting APIs repeatedly.

**Detection (warning signs):**
- No provenance/source tracking in the data model
- All data sources treated identically in code
- No LICENSES.md or license review document
- Bulk downloads without checking API terms of service

**Phase mapping:** Data source integration phase. Provenance tracking must be in the data model from the start.

**Confidence:** HIGH -- licensing is factual and well-documented for these specific sources.

---

### Pitfall 8: Goroutine Leaks in Long-Running Pipeline Batches

**What goes wrong:** A batch job processing 2000 products spawns goroutines for concurrent API calls. When errors occur, some goroutines are not properly cancelled. They hang waiting for a response that will never come (network timeout set too high or missing), or they write to a channel that nobody reads. Over hours of batch processing, leaked goroutines accumulate, consuming memory and file descriptors. The service eventually OOMs or runs out of connections.

**Why it happens:**
- Go goroutines are cheap to create, which encourages creating many of them
- `context.Context` cancellation is not propagated through all code paths
- HTTP client has no timeout or an excessively long timeout (default Go HTTP client has NO timeout)
- Channel operations without `select` and `context.Done()` checks

**Prevention:**
1. Always use `context.WithTimeout` or `context.WithCancel` for every API call. Propagate the context through the entire call chain.
2. Set explicit timeouts on the HTTP client: `&http.Client{Timeout: 30 * time.Second}`. Never use `http.DefaultClient` in production.
3. Use `errgroup.Group` from `golang.org/x/sync/errgroup` for concurrent work with proper cancellation.
4. Implement a worker pool pattern with a bounded number of workers rather than unbounded goroutine spawning.
5. Add goroutine count metrics (runtime.NumGoroutine()) to the dashboard. Alert if count grows over time during a batch.
6. Write tests that run batches with injected errors and verify goroutine count returns to baseline.

**Detection (warning signs):**
- `go func()` calls without context propagation
- HTTP calls using `http.DefaultClient`
- No worker pool pattern -- unbounded goroutine creation
- Memory growth over time during batch processing
- No goroutine count monitoring

**Phase mapping:** Core infrastructure / worker pool design. Must be established before pipeline parallelism is added.

**Confidence:** HIGH -- standard Go concurrency pitfall, extensively documented.

---

### Pitfall 9: PlentyONE API Inconsistencies Between Entity Types

**What goes wrong:** The PlentyONE/plentymarkets REST API is not fully consistent across entity types. Category creation, attribute creation, product creation, variation creation, and text/image endpoints may have different request formats, different error response structures, different pagination approaches, and different rate limit behaviors. Developers build a generic "API caller" that works for categories, then discover it breaks for variations because the endpoint expects a different structure.

**Why it happens:**
- The plentymarkets API evolved over many years and versions. Different endpoints were built at different times by different teams.
- Some endpoints accept JSON body, others use form data. Some return the created entity, others return just an ID. Some use `itemId/variations` nesting, others are flat.
- Documentation may not cover all edge cases or may be slightly out of date for specific fields.

**Prevention:**
1. Build entity-type-specific API clients that implement a common interface but have their own request/response handling. Do not try to make one generic caller handle all entity types.
2. Create integration tests per entity type that exercise the actual PlentyONE API (even against a sandbox) before building the full pipeline.
3. Log full request/response pairs during development (redacting auth tokens). This is invaluable for debugging API behavior.
4. Build response parsing defensively: check for both `{"id": 123}` and `{"entries": [{"id": 123}]}` patterns. Handle both `error` and `errors` keys in error responses.
5. Document discovered API quirks in an internal API_NOTES.md as they are found during development.

**Detection (warning signs):**
- Single generic `callAPI(method, path, body)` function handling all entity types
- Tests only covering one entity type (usually categories, because they are simplest)
- Parsing errors that only occur for specific entity types
- No request/response logging

**Phase mapping:** API client layer. Build and test entity-specific clients before assembling the pipeline.

**Confidence:** MEDIUM -- based on training data about plentymarkets API patterns. Verify against current PlentyONE API docs during implementation.

---

### Pitfall 10: Dashboard Polling Overloads the Service During Batch Processing

**What goes wrong:** The web dashboard polls the Go service for pipeline status updates. During a large batch (2000 products), the dashboard polls every 1-2 seconds. Each poll queries MySQL for current job status across potentially thousands of entity rows. This creates database load that competes with the pipeline's own database writes, slowing down the actual work.

**Why it happens:**
- Dashboard polling interval is set for good UX (frequent updates) without considering the cost
- Status queries are not optimized (e.g., `SELECT * FROM mappings WHERE job_id = X` on a table with 10K+ rows)
- No caching layer between the dashboard and the database
- No separation between "live status" and "detailed view" queries

**Prevention:**
1. Maintain an in-memory job status summary (total products, completed per stage, failed count) that is updated atomically as the pipeline progresses. Dashboard polls read this summary, not the database.
2. Use Server-Sent Events (SSE) instead of polling for real-time updates. Go's `net/http` supports SSE natively. Push updates when state changes rather than polling on a timer.
3. Add appropriate indexes on the mapping tables: `(job_id, stage, status)` composite index.
4. Implement a "summary" endpoint (fast, in-memory) and a "details" endpoint (database query, paginated). Dashboard defaults to summary with on-demand detail drill-down.
5. Rate-limit dashboard API endpoints separately from pipeline API endpoints.

**Detection (warning signs):**
- Dashboard making database queries directly per poll
- No in-memory status cache
- Batch processing slows down when dashboard is open
- `EXPLAIN` on status queries shows full table scans

**Phase mapping:** Dashboard and service API design phase. The status summary pattern should be designed alongside the pipeline, not added after.

**Confidence:** HIGH -- standard issue with monitoring dashboards on active data pipelines.

---

## Minor Pitfalls

---

### Pitfall 11: AI Provider Cost Explosion During Development

**What goes wrong:** During development and testing, developers make hundreds of AI API calls with real providers (OpenAI, Anthropic). Generating product descriptions in 5 languages for 100 test products = 500+ LLM calls. Adding image generation = hundreds more. Monthly AI API bills reach $200-500+ during development alone.

**Prevention:**
1. Build a mock AI provider that returns canned responses for development and testing. Make it the default in development mode.
2. Cache AI responses keyed by (prompt_hash, provider, model). Reuse cached responses during development.
3. Use cheaper models for development (GPT-4o-mini instead of GPT-4o, Haiku instead of Opus).
4. Set hard spending limits on AI provider accounts during development.
5. Only use real AI providers for integration testing and final validation.

**Phase mapping:** AI integration phase. Build the mock provider first, real provider second.

**Confidence:** HIGH.

---

### Pitfall 12: Multilingual Text Generation Quality Varies by Language

**What goes wrong:** AI models generate excellent English product descriptions but produce lower-quality German, Spanish, French, and Italian text. Grammar errors, unnatural phrasing, or mixed-language output appear in non-English text. The product text is customer-facing in PlentyONE, so quality matters.

**Prevention:**
1. Generate each language independently with language-specific prompts, not "translate this English text to German." Direct generation in the target language produces better results than translation.
2. Include language-specific quality examples in prompts (few-shot).
3. Consider a validation step: use a different AI call to review/score generated text quality.
4. Allow per-language regeneration without re-running the entire pipeline.
5. For critical languages (DE is likely most important for PlentyONE users), consider higher-quality models.

**Phase mapping:** AI integration / multilingual text phase.

**Confidence:** MEDIUM -- AI multilingual quality varies by model and is improving rapidly.

---

### Pitfall 13: Not Handling PlentyONE Sandbox vs. Production Differences

**What goes wrong:** Developers build and test against a PlentyONE sandbox/test instance. The sandbox may have different rate limits, different data constraints, or different API behavior than production. Features that work in sandbox fail in production.

**Prevention:**
1. Make the PlentyONE base URL and credentials configurable per environment. Use environment variables or a config file, never hardcoded.
2. Document known sandbox vs. production differences as they are discovered.
3. Run a small validation batch (5-10 products) against production before attempting a full batch.
4. Use the same rate limiting settings for both environments (target production limits).

**Phase mapping:** Configuration / deployment phase.

**Confidence:** MEDIUM -- specific sandbox behavior may vary.

---

### Pitfall 14: Go Module Dependency Conflicts with AI Provider SDKs

**What goes wrong:** AI provider Go SDKs (OpenAI, Anthropic) may depend on different versions of common libraries (protobuf, grpc, http middleware). When using multiple providers, `go mod tidy` produces version conflicts that require manual resolution. Worse, some AI provider Go SDKs are immature or community-maintained rather than official.

**Prevention:**
1. Use HTTP-based API calls with a standard HTTP client rather than provider-specific SDKs where possible. Most AI APIs are simple REST/JSON endpoints.
2. Define a clean provider interface (`type AIProvider interface`) and implement each provider as a thin HTTP wrapper.
3. Avoid importing multiple heavy SDKs simultaneously. If SDK usage is necessary, isolate each behind a build tag or separate internal package.
4. Evaluate SDK maturity before committing: check GitHub stars, recent commits, issue response time.

**Phase mapping:** AI provider integration phase.

**Confidence:** MEDIUM -- Go AI SDKs are evolving rapidly.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Severity | Mitigation |
|-------------|---------------|----------|------------|
| Database schema design | Schema that cannot handle resume (Pitfall 5) | Critical | Design for failure states from day one |
| API client foundation | Token refresh race condition (Pitfall 3) | Critical | Use `oauth2.ReuseTokenSource` or mutex-guarded refresh |
| API client foundation | Rate limiting underestimation (Pitfall 2) | Critical | Centralized rate limiter, test at scale early |
| API client foundation | Entity type inconsistencies (Pitfall 9) | Moderate | Entity-specific clients behind common interface |
| Pipeline orchestration | Orphaned entities (Pitfall 1) | Critical | State machine mapping table, cleanup utility |
| Pipeline orchestration | Goroutine leaks (Pitfall 8) | Moderate | Worker pool, context propagation, `errgroup` |
| AI integration | Validation failures (Pitfall 4) | Critical | Validation layer between AI and API |
| AI integration | Cost explosion (Pitfall 11) | Minor | Mock provider for development |
| AI integration | SDK dependency conflicts (Pitfall 14) | Minor | HTTP clients over SDKs |
| Data source integration | Licensing violations (Pitfall 7) | Moderate | Provenance tracking, license review |
| Image handling | URL expiration (Pitfall 6) | Moderate | Download immediately, store locally |
| Multilingual text | Quality variance (Pitfall 12) | Minor | Per-language generation, not translation |
| Dashboard | Polling overload (Pitfall 10) | Moderate | In-memory status summary, SSE |
| Deployment | Sandbox vs. production (Pitfall 13) | Minor | Environment-aware configuration, validation batches |

---

## Recommended Pitfall-Prevention Order

Based on dependency analysis, address pitfalls in this order during development:

1. **Database schema** (Pitfall 5) -- everything depends on getting the state model right
2. **API client with rate limiting and OAuth** (Pitfalls 2, 3, 9) -- the transport layer must be solid
3. **Pipeline state machine and cleanup** (Pitfall 1) -- before running any real batches
4. **AI validation layer** (Pitfall 4) -- before connecting AI output to the pipeline
5. **Image download pipeline** (Pitfall 6) -- before image upload stage
6. **Data provenance tracking** (Pitfall 7) -- before pulling from public databases
7. **Worker pool and concurrency** (Pitfall 8) -- before scaling to large batches
8. **Dashboard efficiency** (Pitfall 10) -- before users run large batches with dashboard open

---

## Sources

- PlentyONE/plentymarkets REST API documentation (developers.plentymarkets.com) -- MEDIUM confidence, based on training data; verify current rate limits and endpoint behaviors against live docs
- Go standard library and `golang.org/x` packages documentation -- HIGH confidence
- Open Food Facts license (ODbL) and Wikidata license (CC0) -- HIGH confidence, well-established licenses
- OpenAI and Anthropic API documentation -- MEDIUM confidence, API capabilities evolving
- General Go concurrency patterns -- HIGH confidence, well-documented in Go community
- E-commerce API integration patterns -- HIGH confidence, based on extensive domain knowledge

**Note:** WebSearch and WebFetch were unavailable during this research session. PlentyONE-specific rate limits, endpoint behaviors, and sandbox characteristics should be verified against current official documentation during implementation. All PlentyONE-specific claims are marked MEDIUM confidence for this reason.
