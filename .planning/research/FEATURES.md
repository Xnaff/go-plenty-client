# Feature Landscape

**Domain:** E-commerce product data generation + PlentyONE API sync
**Researched:** 2026-03-12
**Overall confidence:** MEDIUM (based on training data for e-commerce PIM/import tooling patterns; no live source verification available during this research session)

---

## Table Stakes

Features users expect. Missing = product feels incomplete or unusable.

### Data Generation

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| AI-powered product text generation | Core value prop -- generating realistic product names, descriptions, bullet points, specs | High | Must support configurable providers (OpenAI, Anthropic). Quality of generated text is the #1 reason someone uses this tool vs manual entry. |
| Multilingual text generation (EN, DE, ES, FR, IT) | Explicitly required. PlentyONE serves EU markets, multi-language is not optional. | Medium | Two approaches: generate in primary language then translate, or generate natively per language. Translation approach is cheaper but lower quality. Native generation per language is better for SEO and fluency. |
| Configurable product niche/type | "Fully generic" is a requirement. User must be able to say "generate electronics" or "generate food products" and get coherent data. | Medium | Needs a niche/template system -- at minimum a prompt-based approach, ideally with niche-specific field mappings (e.g., food has nutrition facts, electronics has specs). |
| Image sourcing from free stock APIs | Products without images are useless in e-commerce. Unsplash/Pexels/Pixabay are the standard free options. | Medium | Must handle attribution requirements per API license. Rate limits vary (Unsplash: 50/hr free tier, Pexels: 200/hr, Pixabay: 100/min). Need fallback strategy when one API is exhausted. |
| AI image generation | Alternative to stock photos for unique product images. User expectation given AI is in the value prop. | High | DALL-E, Midjourney API (if available), Stable Diffusion. Quality varies wildly. Generated images often look artificial for product photography. Should be optional alongside stock photo sourcing. |
| Public database integration (Open Food Facts) | Specified requirement. Real product data grounds AI-generated content in reality. | Medium | Open Food Facts API is well-documented, permissive license (ODbL). ~3M products. Good for food/beverage but useless for other niches. |
| Public database integration (Wikidata) | Broader knowledge base for non-food product types. | Medium | SPARQL endpoint, public domain. Much harder to extract structured product data -- Wikidata is encyclopedic, not commercial. Useful for brand info, material properties, category taxonomies. |

### Pipeline & API Sync

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Strict 6-stage pipeline execution | PlentyONE API requires ordered creation: categories -> attributes -> products -> variations -> images -> text. Violating order = API errors. | High | This is the core engineering challenge. Each stage depends on IDs from prior stages. Must be idempotent (re-runnable after partial failure). |
| Entity ID mapping (local <-> PlentyONE) | Without this, you cannot track what was created, update it, or debug failures. | Medium | MySQL tables mapping local UUIDs to PlentyONE integer IDs. One table per entity type (categories, attributes, products, variations, images, texts). |
| OAuth token management with auto-refresh | PlentyONE uses OAuth2. Tokens expire. Without auto-refresh, any long-running batch job dies mid-execution. | Medium | Standard OAuth2 refresh flow. Store tokens in MySQL or file. Refresh proactively before expiry (not reactively on 401). |
| Rate limiting and backoff | PlentyONE has API rate limits. Thousands of products means thousands of API calls. Without rate limiting, you get blocked. | Medium | Implement token bucket or sliding window. PlentyONE docs specify limits (training data suggests ~100 requests/period but this needs verification). Exponential backoff on 429 responses. |
| Batch processing | Cannot create thousands of products one-at-a-time synchronously. Need concurrent execution within rate limit bounds. | Medium | Worker pool pattern. Configurable concurrency (e.g., 5 concurrent API calls). Pipeline stages are sequential, but items within a stage can be parallelized. |
| Error handling: pause-and-flag | Specified requirement. When a product fails at any pipeline stage, flag it and continue with others rather than aborting the entire batch. | Medium | Per-item status tracking (pending, in_progress, completed, failed, paused). Failed items get error details stored. User reviews and retries. |
| Retry mechanism for transient failures | Network errors, 5xx responses, and timeouts are normal at scale. Silent failure is unacceptable; blind retry without backoff is equally bad. | Low | Distinguish transient (5xx, timeout, rate limit) from permanent (4xx validation) errors. Auto-retry transient with backoff. Flag permanent for user review. |

### User Interface

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| CLI for job management | Specified requirement. Automation-friendly interface for triggering generation, pipeline execution, status checks. | Medium | Subcommands: `generate`, `push`, `status`, `retry`, `config`. Use cobra or similar CLI framework. |
| Web dashboard: pipeline status | Specified requirement. Visual overview of what stage each batch/product is in. | High | Real-time or near-real-time status. Shows per-stage progress (e.g., "Categories: 150/150, Products: 89/200, Variations: 0/500"). |
| Web dashboard: data preview | Users need to see generated data before pushing to PlentyONE. "Generate then review then push" is the expected workflow. | Medium | Table/card view of generated products with fields, images, descriptions. Ability to filter/search. |
| Web dashboard: configuration | Users need to set AI provider keys, PlentyONE credentials, niche settings, language preferences without editing config files. | Medium | Form-based config with validation. Sensitive fields (API keys) masked. Save to DB or config file. |
| Web dashboard: mapping overview | Users need to see which local entities map to which PlentyONE entities, especially for debugging. | Low | Table view with filtering. Links to PlentyONE admin if possible. |
| Logging | Detailed, structured logs for every API call, generation step, and pipeline action. Without this, debugging at scale is impossible. | Low | Structured JSON logging with correlation IDs per batch/product. Log levels (debug, info, warn, error). |

### Data Model

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Category tree support | PlentyONE categories are hierarchical. Products must be assigned to categories. | Medium | Tree structure in local DB. Map to PlentyONE category tree. Support creating new categories or mapping to existing ones. |
| Attribute/property modeling | Products have attributes (color, size) that create variations, and properties (material, weight) that describe features. PlentyONE distinguishes these. | High | Attributes drive variations (e.g., "Red / Large" = one variation). Properties are metadata. Both need local modeling and PlentyONE mapping. |
| Variation support | A single parent product has multiple variations (size/color combos). This is how e-commerce works -- a "T-Shirt" product has 15 variations. | High | Cartesian product of attribute values. Each variation gets its own SKU, price, stock, potentially images. PlentyONE API creates variations as children of products. |
| Price data | Products without prices are incomplete. Even for test/demo data, prices must be realistic. | Low | Generate price ranges appropriate to niche. Support multiple price sets if PlentyONE allows (retail, wholesale). |
| SKU/barcode generation | PlentyONE products need identifiers. | Low | Generate realistic-looking SKUs (pattern-based) and optionally EAN/UPC barcodes. For imported Open Food Facts data, use real barcodes. |

---

## Differentiators

Features that set the product apart. Not expected, but highly valued.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Niche-aware generation templates | Pre-built templates for common niches (food, electronics, fashion, home goods, beauty) with niche-specific fields, attribute sets, and prompt strategies. Most generators are one-size-fits-all. | Medium | E.g., food template includes nutrition facts, allergens, ingredients; fashion template includes material, fit, care instructions. Dramatically improves data quality vs generic prompts. |
| Data quality scoring | Automatically score generated products on completeness, realism, SEO quality, description length, image presence. Flag low-quality items before pushing. | Medium | Scoring rubric: has all required fields? Description > 100 chars? Has images? Price in realistic range? Multilingual texts present? Score 0-100 per product. |
| Dry-run / simulation mode | Run the entire pipeline without actually calling PlentyONE API. Show exactly what would be created, in what order, with what data. | Low | Huge confidence builder. Users can validate the entire batch before committing. Log all "would-be" API calls with payloads. |
| Incremental / resumable pipeline | After a failure, resume from exactly where it stopped rather than re-running everything. | Medium | Requires per-item, per-stage state tracking (already in table stakes via pause-and-flag). The differentiator is making resume seamless -- one command to "continue where we left off." |
| Existing catalog detection | Before creating categories/attributes, check what already exists in PlentyONE and map to it. Avoids duplicate categories or conflicting attribute definitions. | High | Requires GET calls before POST calls. Match by name (fuzzy), external ID, or user-provided mapping. Critical for non-greenfield PlentyONE shops. |
| Product data export (CSV/JSON) | Export generated data to standard formats, not just push to PlentyONE. Useful for review, backup, migration to other platforms. | Low | CSV and JSON export of all generated products with full field set. Could also import from CSV to seed the generator. |
| Batch scheduling / cron | Schedule generation jobs to run at specific times or intervals. Useful for "generate 100 products every night." | Low | Cron-like scheduling via CLI flags or dashboard UI. Store schedule in DB. Run via Go ticker or system cron integration. |
| SEO-optimized text generation | Generate product descriptions optimized for search engines -- keyword density, meta descriptions, structured data hints. | Medium | Requires SEO-aware prompts. Keyword research integration would be HIGH complexity but basic SEO prompting is achievable. Valuable because most AI generators produce generic marketing copy, not SEO copy. |
| Image processing pipeline | Resize, crop, compress, and format images to PlentyONE specifications before upload. Add watermarks or backgrounds. | Medium | PlentyONE likely has image size/format requirements. Processing locally before upload avoids rejection. Libraries: imaging, bimg (Go). |
| Validation against PlentyONE schema | Validate all generated data against PlentyONE's field constraints (max lengths, required fields, allowed values) before attempting to push. | Medium | Prevents wasted API calls on data that will be rejected. Requires modeling PlentyONE's validation rules locally. |
| Progress webhooks / notifications | Send notifications (Slack, email, webhook) when batches complete, fail, or need attention. | Low | Useful for unattended batch runs. Simple webhook POST on state transitions. |
| Multi-shop support | Support pushing to multiple PlentyONE instances from the same tool. | Low | Store multiple PlentyONE credential sets. Select target shop per batch. Separate mapping tables per shop. |
| Seed data augmentation | Take a small set of real products (e.g., 10) and generate hundreds of similar products. Use real products as examples for AI. | Medium | Few-shot prompting with real product data. Much better quality than zero-shot generation. The "example products" approach. |

---

## Anti-Features

Features to explicitly NOT build. Tempting but wrong for this project.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Full PIM (Product Information Management) system | Scope explosion. PIMs like Akeneo, Pimcore are massive multi-year products. This tool generates and pushes data; it does not replace PlentyONE's own product management. | Keep the local data model minimal -- just enough to generate and push. PlentyONE IS the PIM. |
| Two-way sync with PlentyONE | Listening for changes in PlentyONE and syncing back adds enormous complexity (webhooks, conflict resolution, merge logic). Explicitly out of scope per PROJECT.md. | Push-only for v1. If needed later, it's a separate service. |
| Product updates / editing in PlentyONE | Updating existing products requires diff detection, partial updates, and conflict handling. Very different from creation. | v1 is creation-only. Track what was created (via mappings) but don't modify it. |
| Order/payment/inventory management | Completely different domain. PlentyONE handles this natively. | Out of scope. Product data only. |
| Custom AI model training/fine-tuning | Training custom models for product text generation is a research project, not a product feature. | Use commercial APIs (OpenAI, Anthropic) with good prompts. Niche templates get you 80% of the quality at 1% of the effort. |
| Visual product page builder | Tempting to add a "preview how it looks on the shop" feature. This is a frontend rendering problem that belongs in PlentyONE. | Show raw data preview. Maybe render a simple product card for visual QA, but not a full page builder. |
| Multi-platform support (Shopify, WooCommerce, etc.) | "We could also push to Shopify!" Sounds great, adds 3x complexity. Each platform has different APIs, data models, and constraints. | PlentyONE only for v1. Architecture should use a clean interface/adapter pattern so other platforms COULD be added later, but do not build them now. |
| Real-time collaborative editing | Multiple users editing generated products simultaneously is a collaboration feature that requires CRDT/OT, WebSocket sync, etc. | Single-user dashboard. Multiple users can view, but no simultaneous editing. |
| Product recommendation engine | "Since we have all this product data, we could suggest related products!" This is a different product entirely. | Out of scope. PlentyONE or a dedicated recommendation service handles this. |
| Natural language interface ("Generate 50 blue shoes") | Chat-based product generation sounds cool but requires NLU, intent parsing, and a much more complex UX. | Structured forms and config files. CLI flags. Clear, explicit interfaces. |

---

## Feature Dependencies

```
Core Data Model
  |
  +-> Category Tree Support
  |     |
  |     +-> Category Creation in PlentyONE (Pipeline Stage 1)
  |
  +-> Attribute/Property Modeling
  |     |
  |     +-> Attribute Creation in PlentyONE (Pipeline Stage 2)
  |     |
  |     +-> Variation Support
  |           |
  |           +-> Variation Creation in PlentyONE (Pipeline Stage 4)
  |
  +-> AI Text Generation
  |     |
  |     +-> Multilingual Text Generation
  |     |     |
  |     |     +-> Text Push to PlentyONE (Pipeline Stage 7)
  |     |
  |     +-> Niche Templates (enhances quality)
  |     |
  |     +-> SEO-Optimized Generation (enhances quality)
  |
  +-> Image Sourcing (Stock APIs)
  |     |
  |     +-> Image Processing Pipeline (resize/compress)
  |     |     |
  |     |     +-> Image Upload to PlentyONE (Pipeline Stage 6)
  |     |
  |     +-> AI Image Generation (alternative source)
  |
  +-> Price/SKU Generation

OAuth Token Management
  |
  +-> Rate Limiting
  |     |
  |     +-> Batch Processing
  |           |
  |           +-> 6-Stage Pipeline Execution (orchestrates all stages)
  |                 |
  |                 +-> Entity ID Mapping (tracks all created entities)
  |                 |
  |                 +-> Error Handling: Pause-and-Flag
  |                 |     |
  |                 |     +-> Retry Mechanism
  |                 |     |
  |                 |     +-> Incremental/Resumable Pipeline
  |                 |
  |                 +-> Dry-Run Mode (simulates pipeline)
  |                 |
  |                 +-> Existing Catalog Detection (pre-pipeline check)

CLI
  |
  +-> Job Management Commands
  |
  +-> Config Management Commands

Web Dashboard
  |
  +-> Pipeline Status View (requires pipeline + entity mapping)
  |
  +-> Data Preview (requires data model + generation)
  |
  +-> Configuration UI (requires config system)
  |
  +-> Mapping Overview (requires entity mapping)

Logging (cross-cutting, required by everything)

Public DB Integration (Open Food Facts, Wikidata)
  |
  +-> Data Normalization Layer (different sources -> uniform product model)
  |
  +-> Seed Data Augmentation (uses real data as AI examples)
```

### Critical Path

The critical dependency chain that determines build order:

1. **Core data model + MySQL schema** -- everything depends on this
2. **OAuth + HTTP client with rate limiting** -- all API communication depends on this
3. **AI provider abstraction + text generation** -- core value prop
4. **Pipeline orchestration engine** -- the main architectural challenge
5. **Individual pipeline stages** (in order: categories, attributes, products, variations, images, text)
6. **CLI for triggering** -- minimum viable user interface
7. **Dashboard** -- enhancement layer on top of working pipeline

---

## MVP Recommendation

### Must Have for v1 (ordered by implementation priority)

1. **Core data model with MySQL schema** -- foundation for everything
2. **OAuth token management** -- cannot talk to PlentyONE without it
3. **HTTP client with rate limiting and backoff** -- cannot make API calls at scale without it
4. **AI text generation (single provider, e.g., OpenAI)** -- core value; start with one provider, add more later
5. **Category creation pipeline stage** -- first and simplest pipeline stage, proves the pattern
6. **Attribute/property creation pipeline stage** -- second stage, slightly more complex
7. **Product creation pipeline stage** -- core entity
8. **Variation creation pipeline stage** -- most complex stage
9. **Image sourcing from one stock API (Pexels)** -- simplest free API with good rate limits
10. **Image upload pipeline stage** -- ties images to variations
11. **Multilingual text generation + push** -- final pipeline stage
12. **Entity ID mapping** -- tracks everything created
13. **Pause-and-flag error handling** -- essential for debugging at scale
14. **CLI with basic commands** (generate, push, status) -- minimum user interface
15. **Structured logging** -- essential for operations

### Defer to v1.1

- **Web dashboard** -- valuable but CLI is sufficient for initial users
- **AI image generation** -- stock photos are faster and more realistic initially
- **Public database integration** (Open Food Facts, Wikidata) -- adds complexity; pure AI generation works for v1
- **Niche-aware templates** -- default generic prompts work initially
- **Data quality scoring** -- manual review via CLI/dashboard is fine initially
- **Multiple AI providers** -- start with OpenAI, add Anthropic later via clean interface
- **Multiple stock image APIs** -- start with Pexels, add others later

### Defer to v2

- **Existing catalog detection** -- complex, needed only for non-greenfield shops
- **SEO optimization** -- nice-to-have, not essential for data generation
- **Batch scheduling** -- manual triggering is fine initially
- **Multi-shop support** -- single shop is fine initially
- **Product data export** -- PlentyONE IS the export target
- **Seed data augmentation** -- advanced feature, needs real-world usage to design well

---

## Complexity Estimates

| Feature Group | Estimated Effort | Risk Level | Notes |
|---------------|-----------------|------------|-------|
| Core data model + MySQL | 1-2 weeks | Low | Well-understood domain. Schema design is straightforward. |
| OAuth + HTTP client | 1 week | Medium | OAuth2 is standard but PlentyONE-specific quirks need testing against real API. |
| AI provider abstraction | 1-2 weeks | Medium | Interface design matters a lot. OpenAI Go SDK is mature. Prompt engineering is iterative. |
| Pipeline orchestration | 2-3 weeks | **High** | Core architectural challenge. State machine, concurrency, error handling, idempotency all intersect here. |
| Individual pipeline stages (6 total) | 3-4 weeks | Medium | Each stage is moderate, but there are 6 of them and each has PlentyONE API quirks. |
| Image sourcing + processing | 1 week | Low | Well-documented APIs. Standard HTTP calls. |
| Multilingual generation | 1 week | Medium | Prompt engineering for quality multilingual output. Translation vs native generation tradeoff. |
| CLI | 1 week | Low | Cobra is battle-tested. Standard patterns. |
| Web dashboard | 3-4 weeks | Medium | Full-stack work. API server + frontend. Real-time status updates add complexity. |
| Public DB integration | 1-2 weeks | Medium | API integration is easy; normalizing disparate data models into uniform product schema is the challenge. |

**Total estimated effort for MVP (without dashboard):** 10-14 weeks for one developer.
**Total with dashboard:** 14-18 weeks.

---

## Sources

- PlentyONE REST API documentation (developers.plentymarkets.com) -- referenced from project context, not directly verified during this session
- Open Food Facts API (world.openfoodfacts.org/data) -- MEDIUM confidence from training data
- General e-commerce PIM/product data patterns -- based on training data knowledge of Akeneo, Pimcore, Shopify/WooCommerce import tools, and similar domain tools
- AI content generation tools (Jasper, Copy.ai, Writesonic patterns) -- MEDIUM confidence from training data
- Stock photo API documentation (Unsplash, Pexels, Pixabay) -- MEDIUM confidence from training data

**Confidence note:** This research was conducted without live web access. All findings are based on training data (cutoff ~May 2025). Feature categorizations are well-grounded in established e-commerce patterns that are unlikely to have changed, but specific API details (rate limits, endpoints, pricing tiers) should be verified against current documentation before implementation.
