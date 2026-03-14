# PlentyOne Product Generator

A command-line tool and web dashboard that generates complete e-commerce product listings using AI and pushes them into [PlentyONE](https://www.plentymarkets.com/) through its REST API.

PlentyOne generates multilingual product names, descriptions, SEO texts, AI-generated images, stock photos, and realistic pricing — then creates everything in your PlentyONE shop through a 6-stage pipeline that tracks every entity it creates.

## What It Does

1. **Generates** product data using OpenAI (text, images, prices) in 5 languages (EN, DE, ES, FR, IT)
2. **Enriches** products from public databases (Open Food Facts, Wikidata)
3. **Sources** stock photos from Unsplash, Pexels, and Pixabay
4. **Scores** quality of generated content against configurable thresholds
5. **Pushes** everything to PlentyONE through a 6-stage pipeline (categories, attributes, products, variations, images, texts)
6. **Tracks** every local ID to PlentyONE ID mapping in MySQL for full traceability

The pipeline handles failures gracefully — if an item fails, it gets flagged for review while other items continue. You can resume from where it left off.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Database Setup](#database-setup)
- [Configuration](#configuration)
- [Quick Start](#quick-start)
- [CLI Commands](#cli-commands)
- [Web Dashboard](#web-dashboard)
- [How the Pipeline Works](#how-the-pipeline-works)
- [AI Providers](#ai-providers)
- [Stock Photos](#stock-photos)
- [Data Enrichment](#data-enrichment)
- [Quality Scoring](#quality-scoring)
- [Development](#development)
- [Project Structure](#project-structure)
- [Troubleshooting](#troubleshooting)

## Prerequisites

You need the following installed on your machine:

### Go (version 1.25.0 or later)

Go is the programming language this project is built with. Install it from [go.dev/dl](https://go.dev/dl/). After installing, verify it works:

```bash
go version
# Should output: go version go1.25.0 (or higher)
```

### MySQL (version 8.0 or later)

MySQL stores all generated product data, pipeline state, and entity mappings. Install it via:

- **macOS:** `brew install mysql` then `brew services start mysql`
- **Linux (Ubuntu/Debian):** `sudo apt install mysql-server`
- **Windows:** Download from [dev.mysql.com/downloads](https://dev.mysql.com/downloads/mysql/)
- **Docker:** `docker run -d --name mysql -e MYSQL_ROOT_PASSWORD=secret -p 3306:3306 mysql:8`

### Task (task runner)

Task is a modern alternative to Make. Install it from [taskfile.dev/installation](https://taskfile.dev/installation/):

```bash
# macOS
brew install go-task

# Linux
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin

# Go install
go install github.com/go-task/task/v3/cmd/task@latest
```

### Build Tools (required for `task build`)

These two tools are needed to compile templates and CSS when building with `task build`:

**templ** — generates Go code from `.templ` HTML templates:

```bash
go install github.com/a-h/templ/cmd/templ@latest
```

Make sure `~/go/bin` is in your PATH:

```bash
# Bash (~/.bashrc) or Zsh (~/.zshrc)
export PATH="$HOME/go/bin:$PATH"

# Fish (~/.config/fish/config.fish)
fish_add_path ~/go/bin
```

**Tailwind CSS standalone CLI** — compiles Tailwind CSS without needing Node.js. Download the binary for your platform into the `tools/` directory:

```bash
mkdir -p tools

# macOS Apple Silicon (M1/M2/M3/M4)
curl -sL https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-arm64 -o tools/tailwindcss

# macOS Intel
curl -sL https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-x64 -o tools/tailwindcss

# Linux x64
curl -sL https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64 -o tools/tailwindcss

# Linux ARM64
curl -sL https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-arm64 -o tools/tailwindcss

chmod +x tools/tailwindcss
```

> **Skipping `task build`?** If you just want to compile Go without templates/CSS, run `go build -o ./bin/plentyone ./cmd/plentyone` directly. This works because the generated templ code and compiled CSS are committed to the repository.

### Optional Tools

These are only needed if you want to modify database queries or run development tooling:

| Tool | What it does | Install |
|------|-------------|---------|
| **sqlc** | Generates type-safe Go code from SQL queries | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest` |
| **air** | Hot-reload for Go during development | `go install github.com/air-verse/air@latest` |
| **golangci-lint** | Code quality linter | `brew install golangci-lint` or see [golangci-lint.run](https://golangci-lint.run/welcome/install/) |

## Installation

```bash
# Clone the repository
git clone https://github.com/janemig/plentyone.git
cd plentyone

# Download Go dependencies
go mod download

# Install build tools (templ + tailwindcss) — see Prerequisites above
go install github.com/a-h/templ/cmd/templ@latest
mkdir -p tools && curl -sL https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-arm64 -o tools/tailwindcss && chmod +x tools/tailwindcss

# Build the binary
task build
```

This produces a single binary at `./bin/plentyone` that contains everything (including the web dashboard assets). You can copy this binary anywhere and run it.

If you don't have Task installed, or want to skip the template/CSS build step, you can build manually:

```bash
go build -o ./bin/plentyone ./cmd/plentyone
```

## Database Setup

### 1. Create the database and user

Connect to MySQL as root and run:

```sql
CREATE DATABASE plentyone CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'plentyone'@'localhost' IDENTIFIED BY 'your-password-here';
GRANT ALL PRIVILEGES ON plentyone.* TO 'plentyone'@'localhost';
FLUSH PRIVILEGES;
```

### 2. Run migrations

Migrations create all the tables the application needs. They are embedded in the binary, so no separate migration files are needed at runtime.

```bash
# Preview what will happen (no changes made)
./bin/plentyone migrate up --dry-run

# Apply all migrations
./bin/plentyone migrate up
```

This creates 15+ tables for products, categories, attributes, variations, images, texts, pipeline tracking, entity mappings, OAuth tokens, quality scores, and enrichment caching.

To undo the most recent migration:

```bash
./bin/plentyone migrate down
```

## Configuration

PlentyOne reads configuration from three sources (in order of priority):

1. **Environment variables** (highest priority) — prefixed with `PLENTYONE_`
2. **Config file** — YAML format
3. **Built-in defaults** (lowest priority)

### Config File

Copy the example config and edit it:

```bash
cp configs/config.example.yaml config.yaml
```

The application looks for `config.yaml` in these locations (first match wins):

1. Path specified with `--config` flag
2. `./config.yaml` (current directory)
3. `./configs/config.yaml`
4. `~/.plentyone/config.yaml`

### Environment Variables

Create a `.env` file in the project root (loaded automatically on startup):

```bash
cp .env.example .env
```

Every config key maps to an environment variable with the `PLENTYONE_` prefix. Dots become underscores, everything is uppercased:

| Config Key | Environment Variable |
|-----------|---------------------|
| `database.password` | `PLENTYONE_DATABASE_PASSWORD` |
| `ai.api_key` | `PLENTYONE_AI_API_KEY` |
| `api.base_url` | `PLENTYONE_API_BASE_URL` |
| `stock_photos.unsplash_key` | `PLENTYONE_STOCK_PHOTOS_UNSPLASH_KEY` |

### Full Configuration Reference

```yaml
# HTTP server (for web dashboard)
server:
  host: "0.0.0.0"           # Listen address
  port: 8080                 # Listen port

# MySQL connection
database:
  host: "localhost"
  port: 3306
  user: "plentyone"
  password: ""               # Use PLENTYONE_DATABASE_PASSWORD env var
  name: "plentyone"
  max_conns: 25              # Max open connections to MySQL

# Logging
log:
  level: "info"              # debug, info, warn, error
  format: "json"             # json or text (text is more readable for development)

# PlentyONE API connection
api:
  base_url: ""               # Your PlentyONE shop URL, e.g. https://myshop.plentymarkets-cloud01.com
  username: ""               # PlentyONE REST API username
  password: ""               # Use PLENTYONE_API_PASSWORD env var
  rate_limit: 40             # API calls per minute (PlentyONE Basic plan = 40)
  timeout: 60                # HTTP timeout in seconds

# AI text and price generation
ai:
  provider: "mock"           # "mock" for testing, "openai" for real generation
  api_key: ""                # OpenAI API key (use PLENTYONE_AI_API_KEY env var)
  model: "gpt-4o-mini"       # OpenAI model for text generation
  languages:                 # Languages to generate product text in
    - "en"
    - "de"
    - "es"
    - "fr"
    - "it"

# AI image generation
images:
  provider: "mock"           # "mock" for testing, "openai" for real generation
  model: "gpt-image-1"       # OpenAI image model
  quality: "medium"          # low, medium, high (affects cost and quality)
  size: "1024x1024"          # 1024x1024, 1792x1024, 1024x1792
  format: "png"              # png, webp, jpeg
  per_product: 1             # Number of AI-generated images per product

# Stock photo sourcing (from free APIs)
stock_photos:
  enabled: false             # Set to true to enable stock photo sourcing
  providers:                 # Priority order for searching
    - "unsplash"
    - "pexels"
    - "pixabay"
  per_product: 2             # Stock photos per product
  min_width: 800             # Minimum image width in pixels
  min_height: 600            # Minimum image height in pixels
  orientation: "landscape"   # landscape, portrait, square
  unsplash_key: ""           # Get from https://unsplash.com/developers
  pexels_key: ""             # Get from https://www.pexels.com/api/
  pixabay_key: ""            # Get from https://pixabay.com/api/docs/

# Public database enrichment
enrichment:
  enabled: false             # Set to true to enrich products from public databases
  sources:
    - "openfoodfacts"        # Food/beverage product data
    - "wikidata"             # General knowledge (multilingual labels, descriptions)
  cache_ttl: "24h"           # How long to cache API responses

# Quality scoring
quality:
  enabled: true              # Evaluate generated content quality
  min_overall_score: 0.6     # Minimum weighted score (0.0 - 1.0)
  min_text_score: 0.5        # Minimum text completeness score
  min_image_score: 0.4       # Minimum image availability score
  min_data_score: 0.3        # Minimum data richness score
  flag_action: "warn"        # What to do with low-quality products:
                             #   "warn"  - log a warning, continue anyway
                             #   "skip"  - skip low-quality products
                             #   "fail"  - stop generation on first failure

# Pipeline execution
pipeline:
  concurrency: 5             # Parallel API requests per stage
  batch_size: 50             # Items per processing batch
  rate_limit_per_sec: 2.0    # Max PlentyONE API calls per second
```

### Managing Config via CLI

View your current configuration (sensitive values are masked):

```bash
./bin/plentyone config view
```

Set a value (writes to your config file):

```bash
./bin/plentyone config set ai.provider openai
./bin/plentyone config set log.level debug
./bin/plentyone config set pipeline.concurrency 10
```

## Quick Start

### Test with mock provider (no API keys needed)

The mock provider generates deterministic test data without making any API calls. This is the fastest way to verify everything is working:

```bash
# 1. Make sure MySQL is running and migrations are applied
./bin/plentyone migrate up

# 2. Generate 5 test products in the "electronics" niche
./bin/plentyone generate --niche electronics --count 5

# 3. Check what was generated
./bin/plentyone status

# 4. Do a dry run of the push pipeline (no PlentyONE calls)
./bin/plentyone push --job-id 1 --dry-run

# 5. Check pipeline status
./bin/plentyone status --job-id 1
```

### Generate with real AI

```bash
# 1. Set your OpenAI API key
export PLENTYONE_AI_API_KEY="sk-..."
# Or: ./bin/plentyone config set ai.api_key "sk-..."

# 2. Switch to the OpenAI provider
./bin/plentyone config set ai.provider openai
./bin/plentyone config set images.provider openai

# 3. Generate products
./bin/plentyone generate --niche "organic skincare" --count 10
```

### Push to PlentyONE

```bash
# 1. Configure your PlentyONE shop
./bin/plentyone config set api.base_url "https://myshop.plentymarkets-cloud01.com"
export PLENTYONE_API_USERNAME="your-api-user"
export PLENTYONE_API_PASSWORD="your-api-password"

# 2. Push the generated products (replace 1 with your job ID from status)
./bin/plentyone push --job-id 1

# 3. Monitor progress
./bin/plentyone status --job-id 1
```

## CLI Commands

### `generate` — Generate product data

Creates AI-generated product listings and stores them in MySQL.

```bash
./bin/plentyone generate --niche <niche> [--count <n>] [--provider <name>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--niche` | *(required)* | Product niche or category (e.g., "electronics", "organic food", "fashion") |
| `--count` | 10 | Number of products to generate |
| `--provider` | from config | Override AI provider ("mock" or "openai") |

What gets generated per product:
- Product name, descriptions, SEO texts in all configured languages
- SKU, category assignment
- Realistic price based on the niche
- AI-generated product image (if images.provider is set)
- Stock photos with attribution (if stock_photos.enabled is true)
- Enrichment data from public databases (if enrichment.enabled is true)
- Quality score

### `push` — Push to PlentyONE

Executes the 6-stage pipeline to create everything in your PlentyONE shop.

```bash
./bin/plentyone push --job-id <id> [--dry-run] [--resume] [--reset-failed]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--job-id` | *(required)* | Job ID to push (from `generate` output or `status`) |
| `--dry-run` | false | Validate payloads without making API calls |
| `--resume` | false | Resume the latest pipeline run for this job |
| `--reset-failed` | false | Reset failed entity mappings before resuming |

### `status` — View progress

Shows what's been generated and the state of pipeline runs.

```bash
# Show recent jobs
./bin/plentyone status

# Show details for a specific job
./bin/plentyone status --job-id 1

# Show a specific pipeline run
./bin/plentyone status --run-id 5
```

### `config` — Manage settings

```bash
# View all settings (sensitive values masked)
./bin/plentyone config view

# Set a value
./bin/plentyone config set <key> <value>

# Examples
./bin/plentyone config set ai.provider openai
./bin/plentyone config set pipeline.concurrency 10
./bin/plentyone config set log.format text
```

### `migrate` — Database migrations

```bash
# Apply all pending migrations
./bin/plentyone migrate up

# Preview without applying
./bin/plentyone migrate up --dry-run

# Undo the most recent migration
./bin/plentyone migrate down
```

### `serve` — Start web dashboard

```bash
./bin/plentyone serve
# Dashboard available at http://localhost:8080
```

### `version` — Print version

```bash
./bin/plentyone version
```

### Global Flags

| Flag | Description |
|------|-------------|
| `--config <path>` | Path to config file (overrides default search) |

## Web Dashboard

Start the dashboard with `./bin/plentyone serve`, then open `http://localhost:8080` in your browser.

The dashboard provides:

- **Pipeline Status** — Live progress of push operations with per-stage breakdown, progress bars, and error counts. Updates in real time via Server-Sent Events (no manual refresh needed).
- **Data Preview** — Browse generated products, inspect images, multilingual texts, and variations before pushing to PlentyONE.
- **Mappings** — View which local entities are linked to which PlentyONE IDs. Filter by entity type (category, product, variation, etc.) and status.
- **Configuration** — View and edit settings directly from the browser. Sensitive values (API keys, passwords) are masked.
- **Bulk Actions** — Retry or skip failed items directly from the pipeline status view.

## How the Pipeline Works

When you run `push`, the pipeline creates entities in PlentyONE in a strict order. Each stage depends on IDs created by previous stages:

```
Stage 1: Categories      Create product categories
    |
Stage 2: Attributes      Create attributes (Color, Size, etc.) and properties
    |
Stage 3: Products        Create parent products, assign to categories
    |
Stage 4: Variations      Create variations (SKUs) with attributes, set prices
    |
Stage 5: Images          Upload product images
    |
Stage 6: Texts           Create multilingual text (names, descriptions, SEO)
```

### Error Handling

- If an item fails at any stage, it gets **flagged** and the pipeline continues with other items
- Failed items can be **retried** or **skipped** from the CLI or dashboard
- The `--resume` flag picks up exactly where the pipeline left off
- Every entity created in PlentyONE is tracked in the `entity_mappings` table, so nothing gets lost

### Dry Run

Use `--dry-run` to validate everything without making actual PlentyONE API calls. The pipeline simulates the full flow and reports what would be created.

## AI Providers

### Mock Provider

The mock provider returns deterministic test data instantly, without any API calls. It generates placeholder text, a 1x1 pixel test image, and a fixed price of 29.99 EUR.

```yaml
ai:
  provider: "mock"
images:
  provider: "mock"
```

Use this for testing your setup, developing new features, or running CI pipelines.

### OpenAI Provider

Uses OpenAI's API for real AI-generated content:

- **Text & Prices:** Uses the model specified in `ai.model` (default: `gpt-4o-mini`) with structured JSON output
- **Images:** Uses `gpt-image-1` to generate product photography

```yaml
ai:
  provider: "openai"
  api_key: "sk-..."        # Or use PLENTYONE_AI_API_KEY env var
  model: "gpt-4o-mini"     # Or gpt-4o for higher quality
images:
  provider: "openai"
  model: "gpt-image-1"
  quality: "medium"
  size: "1024x1024"
```

**Cost considerations:**
- `gpt-4o-mini` is the most cost-effective for text generation
- Image generation cost depends on size and quality settings
- Each product generates text in all configured languages (5 by default), plus one price generation call

## Stock Photos

Supplement AI-generated images with real product photos from free stock photo APIs. Each provider has its own rate limits that are respected automatically.

```yaml
stock_photos:
  enabled: true
  per_product: 2
  providers:
    - "unsplash"     # 50 requests/hour
    - "pexels"       # 200 requests/hour
    - "pixabay"      # 100 requests/minute
  unsplash_key: ""   # https://unsplash.com/developers
  pexels_key: ""     # https://www.pexels.com/api/
  pixabay_key: ""    # https://pixabay.com/api/docs/
```

Stock photos include attribution metadata (photographer name, source URL) as required by the respective APIs.

You can use any combination of providers. The application searches them in the order listed and collects photos until `per_product` is reached.

## Data Enrichment

Enrich AI-generated products with real-world data from public databases:

```yaml
enrichment:
  enabled: true
  sources:
    - "openfoodfacts"   # Food/beverage products: nutritional data, ingredients, barcodes
    - "wikidata"        # General knowledge: multilingual labels, descriptions, identifiers
  cache_ttl: "24h"      # Cache API responses to avoid repeated lookups
```

- **Open Food Facts** — Best for food and beverage products. Supports barcode lookup and text search.
- **Wikidata** — General-purpose knowledge base. Provides multilingual labels and descriptions for any product type.

Both sources are free and require no API keys. Responses are cached in MySQL to respect rate limits and improve performance.

## Quality Scoring

Every generated product gets a quality score based on three weighted categories:

| Category | Weight | What it checks |
|----------|--------|----------------|
| Text Completeness | 50% | Are all 7 text fields populated in all configured languages? |
| Image Availability | 30% | Does the product have at least one image (AI or stock)? |
| Data Richness | 20% | Was the product enriched with additional data from public databases? |

```yaml
quality:
  enabled: true
  min_overall_score: 0.6    # Weighted average must be at least 0.6
  min_text_score: 0.5
  min_image_score: 0.4
  min_data_score: 0.3
  flag_action: "warn"       # warn | skip | fail
```

**Flag actions:**
- `warn` — Log a warning but continue (default, recommended for initial runs)
- `skip` — Skip low-quality products (they won't be pushed to PlentyONE)
- `fail` — Stop generation immediately if any product fails quality checks

## Development

### Running in Development Mode

Development mode provides hot-reload — the application restarts automatically when you change Go files, and templates regenerate when you change `.templ` files:

```bash
task dev
```

This runs `templ generate --watch` (for HTML templates) and `air` (for Go hot-reload) in parallel.

### Available Task Commands

```bash
task              # List all available tasks
task build        # Build the binary (runs templ + CSS first)
task test         # Run all tests
task test-verbose # Run tests with verbose output
task lint         # Run the Go linter
task generate     # Regenerate sqlc database code (after changing queries.sql)
task templ        # Regenerate templ HTML code (after changing .templ files)
task css          # Rebuild Tailwind CSS
task clean        # Remove build artifacts
task dev          # Start development mode with hot-reload
```

### Modifying Database Queries

If you change SQL queries in `internal/storage/sqlc/queries.sql`:

```bash
task generate    # Regenerates Go code in internal/storage/queries/
```

### Modifying HTML Templates

If you change `.templ` files in `internal/dashboard/views/`:

```bash
task templ       # Regenerates Go code from templates
```

### Modifying CSS

If you change Tailwind classes in `.templ` files:

```bash
task css         # Rebuilds the minified CSS output
```

### Running Tests

```bash
task test          # Quick test run
task test-verbose  # See individual test results
```

### Code Formatting

For `.templ` files: `templ fmt ./internal/dashboard/views/`
For Go files: `gofmt -w .`

## Project Structure

```
plentyone/
├── cmd/plentyone/             # CLI entry point
│   └── main.go                # All commands: generate, push, status, config, serve, migrate
│
├── internal/                  # Core application code (not importable by other projects)
│   ├── app/                   # Application initialization
│   │   ├── config.go          # Configuration structs and loading
│   │   ├── generate.go        # Factory functions for creating providers
│   │   └── logging.go         # Structured logger setup
│   │
│   ├── domain/                # Domain types and enums
│   │   ├── enums.go           # Status, entity type, stage constants
│   │   ├── product.go         # Product, ProductData types
│   │   └── variation.go       # Variation type
│   │
│   ├── generate/              # AI content generation
│   │   ├── generator.go       # Generator and ImageGenerator interfaces
│   │   ├── types.go           # Request/response types for text, images, prices
│   │   ├── prompt.go          # AI prompt construction
│   │   ├── openai/            # OpenAI provider (text, images, prices)
│   │   ├── mock/              # Mock provider for testing
│   │   ├── product/           # Product generation orchestrator
│   │   ├── validate/          # Content validation and sanitization
│   │   └── quality/           # Quality scoring rules and thresholds
│   │
│   ├── imagesource/           # Stock photo APIs
│   │   ├── source.go          # ImageSource interface
│   │   ├── unsplash/          # Unsplash client
│   │   ├── pexels/            # Pexels client
│   │   └── pixabay/           # Pixabay client
│   │
│   ├── enrichment/            # Public database enrichment
│   │   ├── enricher.go        # Enricher interface
│   │   ├── openfoodfacts/     # Open Food Facts client
│   │   └── wikidata/          # Wikidata client
│   │
│   ├── pipeline/              # 6-stage push pipeline
│   │   ├── runner.go          # Pipeline runner and state machine
│   │   ├── stage.go           # Stage interface
│   │   ├── categories.go      # Stage 1: Categories
│   │   ├── attributes.go      # Stage 2: Attributes and properties
│   │   ├── products.go        # Stage 3: Parent products
│   │   ├── variations.go      # Stage 4: Variations + price setting
│   │   ├── images.go          # Stage 5: Image upload
│   │   ├── texts.go           # Stage 6: Multilingual texts
│   │   └── cleanup.go         # Orphan entity cleanup
│   │
│   ├── plenty/                # PlentyONE REST API client
│   │   ├── client.go          # HTTP client with auth chain
│   │   ├── auth.go            # OAuth token management
│   │   ├── ratelimit.go       # Rate limiting transport
│   │   ├── retry.go           # Retry with backoff transport
│   │   ├── types.go           # API request/response types
│   │   ├── categories.go      # Category API methods
│   │   ├── attributes.go      # Attribute API methods
│   │   ├── properties.go      # Property API methods
│   │   ├── products.go        # Product API methods
│   │   ├── variations.go      # Variation + Sales Prices API methods
│   │   ├── images.go          # Image upload API methods
│   │   └── texts.go           # Text API methods
│   │
│   ├── storage/               # Database layer
│   │   ├── db.go              # MySQL connection setup
│   │   ├── migrate.go         # Migration runner
│   │   ├── sqlc/              # SQL query definitions
│   │   │   └── queries.sql    # All SQL queries (source of truth)
│   │   └── queries/           # Generated Go code (do not edit manually)
│   │       ├── queries.sql.go # Generated query functions
│   │       ├── models.go      # Generated model structs
│   │       └── querier.go     # Generated interface
│   │
│   └── dashboard/             # Web dashboard
│       ├── server.go          # Chi router and middleware
│       ├── handlers.go        # HTTP handlers for all pages and API endpoints
│       ├── sse.go             # Server-Sent Events for live updates
│       ├── views/             # Templ HTML templates
│       │   ├── layout.templ   # Base HTML layout (nav, scripts, CSS)
│       │   ├── components.templ # Shared UI components
│       │   ├── pipeline.templ # Pipeline status page
│       │   ├── preview.templ  # Data preview page
│       │   ├── mappings.templ # Entity mappings page
│       │   └── config.templ   # Configuration page
│       └── static/            # Static assets (embedded in binary)
│           ├── css/           # Tailwind CSS
│           └── js/            # HTMX and SSE extension
│
├── migrations/                # SQL migration files (embedded in binary)
│   ├── 000001_initial.up.sql
│   ├── 000001_initial.down.sql
│   ├── 000002_oauth_tokens.up.sql
│   ├── 000002_oauth_tokens.down.sql
│   ├── 000003_quality_enrichment.up.sql
│   └── 000003_quality_enrichment.down.sql
│
├── configs/
│   └── config.example.yaml    # Example configuration file
│
├── Taskfile.yml               # Task runner configuration
├── go.mod                     # Go module definition
├── sqlc.yaml                  # SQL code generation config
├── .air.toml                  # Hot-reload config for development
├── .golangci.yml              # Linter configuration
├── .env.example               # Example environment variables
└── README.md                  # This file
```

## Troubleshooting

### "go: go.mod requires go >= 1.25.0"

Your Go version is too old. Download Go 1.25.0 or later from [go.dev/dl](https://go.dev/dl/).

### "Error 1045: Access denied for user 'plentyone'@'localhost'"

MySQL credentials are wrong. Check your `database.user` and `database.password` config, or verify the MySQL user exists:

```sql
SELECT user, host FROM mysql.user WHERE user = 'plentyone';
```

### "dial tcp: connect: connection refused" (database)

MySQL isn't running. Start it:

```bash
# macOS
brew services start mysql

# Linux
sudo systemctl start mysql

# Docker
docker start mysql
```

### "no such table" errors

Migrations haven't been applied. Run:

```bash
./bin/plentyone migrate up
```

### OpenAI API errors

- **401 Unauthorized:** Your API key is invalid. Check `PLENTYONE_AI_API_KEY`.
- **429 Rate Limited:** You're sending too many requests. Lower `pipeline.concurrency` or `pipeline.rate_limit_per_sec`.
- **500 Server Error:** OpenAI is having issues. The retry transport will automatically retry with backoff.

### Pipeline stuck or partially completed

Resume from where it left off:

```bash
./bin/plentyone push --job-id <id> --resume
```

If specific items are permanently failing, skip them:

```bash
./bin/plentyone push --job-id <id> --resume --reset-failed
```

Or use the web dashboard's bulk actions to retry or skip individual items.

### "task: command not found"

Install the Task runner — see [Prerequisites](#prerequisites).

### Dashboard not loading

Make sure the binary was built with `task build` (which includes the CSS and templ compilation steps). If you built with plain `go build`, the dashboard assets may be missing.

## License

See [LICENSE](LICENSE) for details.
