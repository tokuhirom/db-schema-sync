# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make build              # Build the binary
make test               # Run unit tests
make test-integration   # Run integration tests (requires Docker)
make lint               # Run golangci-lint

# Run a single test
go test -v -run TestFunctionName ./cmd/db-schema-sync/...

# Run tests with specific build tag
go test -tags=integration ./cmd/db-schema-sync/...
```

## Architecture

**db-schema-sync** synchronizes PostgreSQL schemas from S3 using [psqldef](https://github.com/k0kubun/sqldef).

### Core Flow
1. Poll S3 for schema files at `s3://bucket/path-prefix/version/schema.sql`
2. Find the latest version using semantic version comparison
3. Acquire PostgreSQL advisory lock (prevents concurrent applies)
4. Apply schema via psqldef subprocess
5. Create completion marker in S3
6. Execute lifecycle hooks

### Key Components (all in `cmd/db-schema-sync/`)

- **main.go**: CLI definition (kong), subcommands (watch/apply/plan/fetch-completed), `runSync()` core logic
- **lock.go**: PostgreSQL advisory lock for concurrency control
- **metrics.go**: Prometheus metrics for watch mode

### Test Structure
- `*_unit_test.go` - Unit tests (no external dependencies, use `//go:build !integration`)
- `*_test.go` without build tag or with `//go:build integration` - Integration tests (require Docker/testcontainers)

### S3 Schema Organization
```
s3://bucket/path-prefix/
  v1/schema.sql
  v1/completed        # marker file created after successful apply
  v1/exported.sql     # optional: exported schema after apply
  v2/schema.sql
  ...
```

## Code Style

- Comments must be written in English
- Use `github.com/lib/pq` for PostgreSQL connections (advisory lock)
- Use `github.com/alecthomas/kong` for CLI parsing
- Mock S3Client interface for unit tests
