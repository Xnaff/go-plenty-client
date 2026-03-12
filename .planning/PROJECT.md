# PlentyOne Product Generator

## What This Is

A Go-based service with CLI and web dashboard that generates e-commerce product data using AI and public databases, then pushes it into PlentyONE through their REST API. It manages the complex multi-step creation pipeline (categories → attributes → products → variations → images → multilingual text) and tracks all mappings between local and PlentyONE IDs in MySQL.

## Core Value

Reliably generate and push thousands of complete, multilingual product listings into PlentyONE through its multi-step API, with full traceability of every entity created.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Generate product data using configurable AI providers (text + images)
- [ ] Pull product data from public databases (Open Food Facts, Wikidata, others)
- [ ] Push data into PlentyONE via strict 6-stage pipeline
- [ ] Track all entity mappings in MySQL (local ID ↔ PlentyONE ID)
- [ ] Web dashboard with pipeline status, data preview, configuration, and mapping overview
- [ ] CLI commands for triggering and managing jobs
- [ ] Multilingual text generation (EN, DE, ES, FR, IT)
- [ ] OAuth token management with automatic refresh
- [ ] Rate limiting and backoff for PlentyONE API
- [ ] Pause-and-flag error handling for failed pipeline steps
- [ ] Support variation products with attributes
- [ ] Fully generic — works for any product type/niche

### Out of Scope

- Mobile app — web dashboard is sufficient
- Real-time PlentyONE webhooks — push-only, not listening for changes
- Product updates/sync — initial creation pipeline only for v1
- Payment or order management — product data only

## Context

**PlentyONE API creation sequence (strict ordering):**
1. Create categories
2. Create attributes and properties
3. Create parent product (minimalistic data)
4. Create variations with attributes
5. Connect properties to variations
6. Upload images
7. Add multilingual text (EN, DE, ES, FR, IT)

Each step depends on IDs returned from previous steps. A single "product" requires multiple API calls across these stages.

**Data sources:**
- AI providers (configurable): Text generation for product names, descriptions, properties. Image generation for product photos.
- Public databases: Open Food Facts (food/beverage), Wikidata (general knowledge), others TBD during research.
- Free stock image APIs: Unsplash, Pexels, Pixabay for realistic product photography.

**Scale:** Thousands of products. Requires batch processing, rate limiting, and robust error handling.

**Auth:** PlentyONE uses OAuth with token refresh. Need persistent token management.

## Constraints

- **Language**: Go — chosen by user
- **Database**: MySQL — for mapping tables and pipeline state
- **API**: PlentyONE REST API — multi-step creation, OAuth auth
- **Scale**: Must handle thousands of products — needs batch orchestration
- **Licensing**: Only public databases with permissive/open licenses
- **Languages**: Product text in 5 languages (EN, DE, ES, FR, IT)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go as primary language | User preference | — Pending |
| MySQL for mapping/state | User preference, relational fits mapping tables | — Pending |
| Configurable AI providers | Flexibility to swap between OpenAI, Anthropic, others | — Pending |
| Pause-and-flag on errors | Better than silent retry for thousands of products — lets user review failures | — Pending |
| Service + CLI + Dashboard | Dashboard for monitoring, CLI for automation/scripting | — Pending |

---
*Last updated: 2026-03-12 after initialization*
