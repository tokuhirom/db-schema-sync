# DB Schema Sync

A tool to synchronize database schemas from S3 using psqldef.

## Features

- Periodically polls S3 for schema file updates
- Automatically applies new schema versions when detected
- Uses psqldef for safe schema migrations
- Lifecycle hooks for startup, success, and error notifications
- Flexible configuration via environment variables or CLI flags
- Dockerized for easy deployment

## License

This project is licensed under the MIT License.

## Usage

### How it works

This tool continuously monitors an S3 bucket for schema file updates. It periodically polls the specified S3 location and automatically applies any new schema versions it finds. This approach is similar to how dewy works for continuous delivery.

Schema files are expected to be organized in S3 with the following structure:
```
s3://bucket/path-prefix/version/schema.sql
```

Where:
- `path-prefix` is specified with the `--path-prefix` flag or `PATH_PREFIX` env var
- `version` is a timestamp or version identifier (sorted alphabetically)
- `schema.sql` is the schema file name specified with the `--schema-file` flag or `SCHEMA_FILE` env var

The tool finds all versions of the schema file and applies the one with the lexicographically highest version identifier.

When a new schema file is detected:
1. The tool downloads the latest schema file
2. Applies the schema using psqldef
3. Creates a completion marker file in S3
4. Executes the on-apply-succeeded hook (if specified)

### Configuration

All options can be set via **environment variables** or **CLI flags**. CLI flags take precedence over environment variables.

#### S3 Settings

| Flag | Environment Variable | Description | Required |
|------|---------------------|-------------|----------|
| `--s3-bucket` | `S3_BUCKET` | S3 bucket name containing schema files | Yes |
| `--path-prefix` | `PATH_PREFIX` | S3 path prefix (e.g., "schemas/") | Yes |
| `--schema-file` | `SCHEMA_FILE` | Schema file name (default: "schema.sql") | No |

#### Database Settings

| Flag | Environment Variable | Description | Required |
|------|---------------------|-------------|----------|
| `--db-host` | `DB_HOST` | Database host | Yes |
| `--db-port` | `DB_PORT` | Database port | Yes |
| `--db-user` | `DB_USER` | Database user | Yes |
| `--db-password` | `DB_PASSWORD` | Database password | Yes |
| `--db-name` | `DB_NAME` | Database name | Yes |

#### Polling Settings

| Flag | Environment Variable | Description | Default |
|------|---------------------|-------------|---------|
| `--interval` | `INTERVAL` | Polling interval | 1m |
| `--completed-file` | `COMPLETED_FILE` | Completion marker file name | completed |

#### Lifecycle Hooks

| Flag | Environment Variable | Description |
|------|---------------------|-------------|
| `--on-start` | `ON_START` | Command to run when the process starts |
| `--on-s3-fetch-error` | `ON_S3_FETCH_ERROR` | Command to run when S3 fetch fails 3 times consecutively |
| `--on-apply-failed` | `ON_APPLY_FAILED` | Command to run when schema application fails |
| `--on-apply-succeeded` | `ON_APPLY_SUCCEEDED` | Command to run after schema is successfully applied |

#### AWS Credentials

AWS credentials are handled by the AWS SDK and can be configured via:
- `AWS_ACCESS_KEY_ID`: AWS access key ID
- `AWS_SECRET_ACCESS_KEY`: AWS secret access key
- `AWS_SESSION_TOKEN`: AWS session token (if using temporary credentials)
- `AWS_DEFAULT_REGION`: AWS region (optional, defaults to us-east-1)
- IAM roles (when running on EC2, ECS, or EKS)

### Examples

#### Using environment variables:

```bash
export S3_BUCKET=my-bucket
export PATH_PREFIX=schemas/
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=user
export DB_PASSWORD=pass
export DB_NAME=mydb
export INTERVAL=30s
export ON_APPLY_SUCCEEDED="curl -X POST https://my-api/notify"
export ON_APPLY_FAILED="curl -X POST https://my-api/alert"

db-schema-sync
```

#### Using CLI flags:

```bash
db-schema-sync \
  --s3-bucket my-bucket \
  --path-prefix schemas/ \
  --db-host localhost \
  --db-port 5432 \
  --db-user user \
  --db-password pass \
  --db-name mydb \
  --interval 30s \
  --on-apply-succeeded "curl -X POST https://my-api/notify" \
  --on-apply-failed "curl -X POST https://my-api/alert"
```

#### Using Docker with environment variables:

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
  -e ON_APPLY_SUCCEEDED="curl -X POST https://my-api/notify" \
  -e ON_APPLY_FAILED="curl -X POST https://my-api/alert" \
  ghcr.io/tokuhirom/db-schema-sync:latest
```

#### Mixing environment variables and CLI flags:

```bash
# Set secrets via environment variables
export DB_PASSWORD=secret
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...

# Pass other options via CLI flags
db-schema-sync \
  --s3-bucket my-bucket \
  --path-prefix schemas/ \
  --db-host localhost \
  --db-port 5432 \
  --db-user user \
  --db-name mydb
```

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
