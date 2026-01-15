# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Fastly Compute@Edge application written in Go that displays wind turbine performance data from the Vensys API. The application runs as a WebAssembly module on Fastly's edge computing platform.

## Architecture

**Dual Build System**: The project uses different Go compilers for development vs production:
- **Development**: Uses standard `go` compiler (faster iteration, full stdlib support)
- **Production**: Uses TinyGo compiler (smaller binary size, limited stdlib)
- This matters because TinyGo doesn't support all stdlib packages (e.g., `encoding/json` doesn't work)
- Currently using `valyala/fastjson` instead of stdlib `encoding/json` for TinyGo compatibility

**Data Flow**:
- External API: Vensys wind turbine API (requires ApiKey and TID headers)
- Caching: Fastly's CDN cache (10-minute TTL on API requests)
- Storage: Fastly KV Store (`vensys-data`) for historical data persistence
- Secrets: Fastly Secret Store (`vensys-secret`) for API key management
- Template: Embedded `index.html.tmpl` rendered with pongo2 templating engine

**Routes** (in main.go:65-109):
- `/` - Main dashboard displaying current and 30-day performance data
- `/favicon.ico` - Embedded favicon
- `/last30` - JSON endpoint for 30-day performance data
- `/year` - JSON endpoint for yearly performance data (hardcoded to 2024)
- `/history?month=N` - Historical data for a specific month (2025)

**API Integration**:
- Backend: `api.vensys.de:8443`
- Authentication: Uses ApiKey header from secret store and TID constant (277)
- Two endpoints consumed:
  - `/api/v1.0/Customer/Performance` - Power and energy data
  - `/api/v1.0/Customer/MeanData` - Generator speed and other metrics

## Development Commands

### Running Locally
```bash
make dev
# Starts local Fastly Compute server on http://127.0.0.1:7676
# Uses fastly.dev.toml configuration with standard Go compiler
```

### Building
The build process is handled by Fastly CLI using the configuration in `fastly.toml`:
- Development: `go build -o bin/main.wasm ./` (uses standard Go)
- Production: Would use TinyGo (commented out in current branch)

### Deployment
Automatic deployment to Fastly Compute happens on push to `main` branch via GitHub Actions (`.github/workflows/fastly.yaml`). The workflow:
1. Sets up Go and TinyGo toolchains
2. Uses `fastly/compute-actions@v11` to build and deploy
3. Requires `FASTLY_API_TOKEN` secret

## Important Files

- `main.go` - All application logic (noted as "a mess" in README)
- `fastly.toml` - Production Fastly configuration (service_id: nqSA4bttH0mRZ7Ab3mmcC5)
- `fastly.dev.toml` - Local development configuration
- `index.html.tmpl` - Pongo2 template for dashboard UI
- `secret.json` - Local development API key (not in version control)
- `data.json` - Local KV store for development

## Setup Requirements

1. Install [Fastly CLI](https://github.com/fastly/cli)
2. Install [Go](https://go.dev/doc/install)
3. Add API key to `secret.json` file
4. Run `make dev`

## Current Branch Context

Branch: `big-go` - Experimenting with using standard Go instead of TinyGo in production. The `fastly.toml` has the TinyGo build command commented out and uses standard Go build instead.
