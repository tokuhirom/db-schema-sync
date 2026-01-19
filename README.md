# DB Schema Sync

A tool to synchronize database schemas from S3 using psqldef.

## Features

- Periodically polls S3 for schema file updates (watch mode)
- Single-shot schema application (apply mode)
- Plan mode for offline schema comparison (plan mode)
- Fetch latest completed schema from S3 (fetch-completed mode)
- Export schema after apply and upload to S3
- Semantic version sorting for schema versions
- Uses psqldef for safe schema migrations
- Lifecycle hooks for startup, success, and error notifications
- Flexible configuration via environment variables or CLI flags
- S3-compatible storage support (Sakura Cloud, MinIO, etc.)
- Dockerized for easy deployment

## License

This project is licensed under the MIT License.

## Usage

### Subcommands

```
db-schema-sync watch            # Run in daemon mode, continuously polling for schema updates
db-schema-sync apply            # Apply schema once and exit
db-schema-sync plan             # Show DDL changes between S3 schema and local file (like terraform plan)
db-schema-sync fetch-completed  # Fetch latest completed schema from S3
```

### How it works

This tool monitors an S3 bucket for schema file updates. Schema files are expected to be organized in S3 with the following structure:
```
s3://bucket/path-prefix/version/schema.sql
```

Where:
- `path-prefix` is specified with the `--path-prefix` flag or `PATH_PREFIX` env var
- `version` is a semantic version (v1, v2, v1.0.0, etc.) or timestamp (20240101120000)
- `schema.sql` is the schema file name specified with the `--schema-file` flag or `SCHEMA_FILE` env var

The tool finds all versions of the schema file and applies the one with the highest version (using semantic version comparison).

When a new schema file is detected:
1. The tool downloads the latest schema file
2. Applies the schema using psqldef
3. (Optional) Exports the current database schema and uploads to S3 as `exported.sql`
4. Creates a completion marker file in S3
5. Executes the on-apply-succeeded hook (if specified)

### Configuration

All options can be set via **environment variables** or **CLI flags**. CLI flags take precedence over environment variables.

#### Global S3 Settings

| Flag | Environment Variable | Description | Required |
|------|---------------------|-------------|----------|
| `--s3-bucket` | `S3_BUCKET` | S3 bucket name containing schema files | Yes |
| `--s3-endpoint` | `S3_ENDPOINT` | Custom S3 endpoint URL for S3-compatible storage | No |
| `--path-prefix` | `PATH_PREFIX` | S3 path prefix (e.g., "schemas/") | Yes |
| `--schema-file` | `SCHEMA_FILE` | Schema file name (default: "schema.sql") | No |
| `--completed-file` | `COMPLETED_FILE` | Completion marker file name (default: "completed") | No |

#### Database Settings (watch/apply only)

| Flag | Environment Variable | Description | Required |
|------|---------------------|-------------|----------|
| `--db-host` | `DB_HOST` | Database host | Yes |
| `--db-port` | `DB_PORT` | Database port | Yes |
| `--db-user` | `DB_USER` | Database user | Yes |
| `--db-password` | `DB_PASSWORD` | Database password | Yes |
| `--db-name` | `DB_NAME` | Database name | Yes |

#### Export Settings (watch/apply only)

| Flag | Environment Variable | Description | Default |
|------|---------------------|-------------|---------|
| `--export-after-apply` | `EXPORT_AFTER_APPLY` | Export schema after successful apply and upload to S3 as `exported.sql` | false |

#### Concurrency Control (watch/apply only)

| Flag | Environment Variable | Description | Default |
|------|---------------------|-------------|---------|
| `--skip-lock` | `SKIP_LOCK` | Skip advisory lock (not recommended for production) | false |

**Advisory Lock:**

When multiple instances of db-schema-sync run against the same database, they use PostgreSQL Advisory Locks to ensure only one instance applies the schema at a time. This prevents race conditions and duplicate schema applications.

- Uses `pg_try_advisory_lock()` for non-blocking lock acquisition
- If another process holds the lock, the current process skips the apply and logs "Another process is applying schema, skipping"
- Lock is automatically released when the connection closes (crash-safe)
- Lock scope is per-database, so different databases can be updated concurrently

**Note:** Use `--skip-lock` only for testing or when you're certain only one instance will run.

#### Watch Mode Settings

| Flag | Environment Variable | Description | Default |
|------|---------------------|-------------|---------|
| `--interval` | `INTERVAL` | Polling interval | 1m |
| `--metrics-addr` | `METRICS_ADDR` | Metrics endpoint address (e.g., `:9090`). Disabled if not set | (disabled) |

#### Prometheus Metrics (watch only)

When `--metrics-addr` is set, the tool exposes Prometheus metrics on the specified address.

**Endpoints:**
- `/metrics` - Prometheus metrics
- `/health` - Health check (returns 200 OK)

**Exposed Metrics:**

| Metric Name | Type | Description |
|-------------|------|-------------|
| `db_schema_sync_apply_total` | Counter | Total number of schema apply attempts |
| `db_schema_sync_apply_success_total` | Counter | Total number of successful schema applies |
| `db_schema_sync_apply_error_total` | Counter | Total number of failed schema applies |
| `db_schema_sync_s3_fetch_total` | Counter | Total number of S3 fetch attempts |
| `db_schema_sync_s3_fetch_error_total` | Counter | Total number of S3 fetch errors |
| `db_schema_sync_consecutive_failures` | Gauge | Current number of consecutive failures |
| `db_schema_sync_last_apply_timestamp_seconds` | Gauge | Unix timestamp of the last successful schema apply |
| `db_schema_sync_process_start_time_seconds` | Gauge | Unix timestamp when the process started |
| `db_schema_sync_last_applied_version_info` | Gauge | Information about the last applied version (with `version` label) |

In addition, the Go Prometheus client automatically exposes `process_*` and `go_*` metrics.

#### Lifecycle Hooks (watch/apply)

| Flag | Environment Variable | Description |
|------|---------------------|-------------|
| `--on-start` | `ON_START` | Command to run when the process starts (watch only) |
| `--on-s3-fetch-error` | `ON_S3_FETCH_ERROR` | Command to run when S3 fetch fails 3 times consecutively (watch only) |
| `--on-before-apply` | `ON_BEFORE_APPLY` | Command to run before schema application starts |
| `--on-apply-failed` | `ON_APPLY_FAILED` | Command to run when schema application fails |
| `--on-apply-succeeded` | `ON_APPLY_SUCCEEDED` | Command to run after schema is successfully applied |

**Hook Environment Variables:**

When hook commands are executed, the following environment variables are available:

| Variable | Description | Hooks |
|----------|-------------|-------|
| `DB_SCHEMA_SYNC_S3_BUCKET` | S3 bucket name | All |
| `DB_SCHEMA_SYNC_PATH_PREFIX` | S3 path prefix | All |
| `DB_SCHEMA_SYNC_SCHEMA_FILE` | Schema file name | All |
| `DB_SCHEMA_SYNC_COMPLETED_FILE` | Completion marker file name | All |
| `DB_SCHEMA_SYNC_VERSION` | Schema version being applied | on-before-apply, on-apply-failed, on-apply-succeeded, on-s3-fetch-error |
| `DB_SCHEMA_SYNC_ERROR` | Error message | on-apply-failed, on-s3-fetch-error |
| `DB_SCHEMA_SYNC_APP_VERSION` | db-schema-sync version | All |
| `DB_SCHEMA_SYNC_STDOUT` | psqldef stdout output | on-apply-failed |
| `DB_SCHEMA_SYNC_STDERR` | psqldef stderr output | on-apply-failed |
| `DB_SCHEMA_SYNC_DRY_RUN` | psqldef --dry-run output (DDL to be applied) | on-before-apply |

Example hook script:
```bash
#!/bin/bash
# notify-slack.sh - Send notification to Slack
curl -X POST https://hooks.slack.com/services/xxx \
  -H 'Content-Type: application/json' \
  -d "{\"text\": \"Schema apply failed for version $DB_SCHEMA_SYNC_VERSION: $DB_SCHEMA_SYNC_ERROR\"}"
```

#### AWS Credentials

AWS credentials are handled by the AWS SDK and can be configured via:
- `AWS_ACCESS_KEY_ID`: AWS access key ID
- `AWS_SECRET_ACCESS_KEY`: AWS secret access key
- `AWS_SESSION_TOKEN`: AWS session token (if using temporary credentials)
- `AWS_DEFAULT_REGION`: AWS region (optional, defaults to us-east-1)
- IAM roles (when running on EC2, ECS, or EKS)

### Examples

#### Watch mode (daemon):

```bash
db-schema-sync watch \
  --s3-bucket my-bucket \
  --path-prefix schemas/ \
  --db-host localhost \
  --db-port 5432 \
  --db-user user \
  --db-password pass \
  --db-name mydb \
  --interval 30s \
  --on-apply-succeeded "curl -X POST https://my-api/notify"
```

#### Watch mode with Prometheus metrics:

```bash
db-schema-sync watch \
  --s3-bucket my-bucket \
  --path-prefix schemas/ \
  --db-host localhost \
  --db-port 5432 \
  --db-user user \
  --db-password pass \
  --db-name mydb \
  --metrics-addr :9090
```

Metrics are available at `http://localhost:9090/metrics` and health check at `http://localhost:9090/health`.

#### Apply mode (single-shot):

```bash
db-schema-sync apply \
  --s3-bucket my-bucket \
  --path-prefix schemas/ \
  --db-host localhost \
  --db-port 5432 \
  --db-user user \
  --db-password pass \
  --db-name mydb
```

#### Apply mode with schema export:

```bash
db-schema-sync apply \
  --s3-bucket my-bucket \
  --path-prefix schemas/ \
  --db-host localhost \
  --db-port 5432 \
  --db-user user \
  --db-password pass \
  --db-name mydb \
  --export-after-apply
```

This exports the database schema after successful apply and uploads it to S3 as `exported.sql` in the same directory as `schema.sql`.

#### Plan mode (show DDL changes):

```bash
db-schema-sync plan \
  --s3-bucket my-bucket \
  --path-prefix schemas/ \
  schema.sql
```

This shows the DDL changes that would be applied when migrating from the current S3 schema (`exported.sql` or `schema.sql`) to your local `schema.sql` file. Uses psqldef's offline mode, so no database connection is required. Useful for reviewing changes before creating a PR (like `terraform plan`).

#### Fetch completed schema from S3:

```bash
# Output to stdout
db-schema-sync fetch-completed \
  --s3-bucket my-bucket \
  --path-prefix schemas/

# Save to file
db-schema-sync fetch-completed \
  --s3-bucket my-bucket \
  --path-prefix schemas/ \
  -o current-schema.sql
```

This fetches the latest completed schema (`exported.sql` or `schema.sql`) from S3.

#### Using environment variables:

```bash
export S3_BUCKET=my-bucket
export PATH_PREFIX=schemas/
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=user
export DB_PASSWORD=pass
export DB_NAME=mydb

db-schema-sync watch --interval 30s
```

#### Using Docker:

```bash
docker run \
  -e S3_BUCKET=my-bucket \
  -e PATH_PREFIX=schemas/ \
  -e DB_HOST=localhost \
  -e DB_PORT=5432 \
  -e DB_USER=user \
  -e DB_PASSWORD=pass \
  -e DB_NAME=mydb \
  -e INTERVAL=30s \
  ghcr.io/tokuhirom/db-schema-sync:latest watch
```

#### Using S3-compatible storage (e.g., Sakura Cloud, MinIO):

```bash
db-schema-sync watch \
  --s3-endpoint https://s3.isk01.sakurastorage.jp \
  --s3-bucket my-bucket \
  --path-prefix schemas/ \
  --db-host localhost \
  --db-port 5432 \
  --db-user user \
  --db-password pass \
  --db-name mydb
```

## Version Formats

The tool supports semantic version comparison. Supported formats:
- Simple versions: `v1`, `v2`, `v10` (v10 > v9)
- Semver: `1.0.0`, `2.0.0`, `1.10.0`
- Semver with v prefix: `v1.0.0`, `v2.0.0`
- Timestamps: `20240101120000`

## Development

### Build

```bash
make build
```

### Test

```bash
# Run unit tests
make test

# Run integration tests (requires Docker)
make test-integration
```

### Lint

```bash
make lint
```
