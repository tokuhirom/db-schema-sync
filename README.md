# DB Schema Sync

A tool to synchronize database schemas from S3 using psqldef.

## Features

- Periodically polls S3 for schema file updates (watch mode)
- Single-shot schema application (apply mode)
- Diff between S3 schema and local file (diff mode)
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
db-schema-sync watch   # Run in daemon mode, continuously polling for schema updates
db-schema-sync apply   # Apply schema once and exit
db-schema-sync diff    # Show diff between S3 schema and local file
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
3. Creates a completion marker file in S3
4. Executes the on-apply-succeeded hook (if specified)

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

#### Diff mode (compare with local file):

```bash
db-schema-sync diff \
  --s3-bucket my-bucket \
  --path-prefix schemas/ \
  schema.sql
```

This shows the diff between the latest completed schema in S3 and your local `schema.sql` file. Useful for reviewing changes before creating a PR.

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
