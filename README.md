# DB Schema Sync

A tool to synchronize database schemas from S3 using psqldef.

## Features

- Periodically polls S3 for schema file updates
- Automatically applies new schema versions when detected
- Uses psqldef for safe schema migrations
- Lifecycle hooks for startup, success, and error notifications
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
- `path-prefix` is specified with the `-path-prefix` flag
- `version` is a timestamp or version identifier (sorted alphabetically)
- `schema.sql` is the schema file name specified with the `-schema-file` flag

The tool finds all versions of the schema file and applies the one with the lexicographically highest version identifier.

When a new schema file is detected:
1. The tool downloads the latest schema file
2. Applies the schema using psqldef
3. Creates a completion marker file in S3
4. Executes the on-apply-succeeded hook (if specified)

### Environment Variables

- `S3_BUCKET`: S3 bucket name containing schema files
- `DB_HOST`: Database host
- `DB_PORT`: Database port
- `DB_USER`: Database user
- `DB_PASSWORD`: Database password
- `DB_NAME`: Database name
- AWS credentials (automatically used by AWS SDK):
  - `AWS_ACCESS_KEY_ID`: AWS access key ID
  - `AWS_SECRET_ACCESS_KEY`: AWS secret access key
  - `AWS_SESSION_TOKEN`: AWS session token (if using temporary credentials)
  - `AWS_DEFAULT_REGION`: AWS region (optional, defaults to us-east-1)

### Command Line Options

- `-path-prefix`: S3 path prefix (e.g., "schemas/" or "prod/schemas/")
- `-schema-file`: Schema file name within the path prefix (default: "schema.sql")
- `-interval`: Polling interval (default: 1m0s)
- `-completed-file`: Name of completion marker file to create alongside schema file (default: "completed")

#### Lifecycle Hooks

- `-on-start`: Command to run when the process starts
- `-on-s3-fetch-error`: Command to run when S3 fetch fails 3 times consecutively
- `-on-apply-failed`: Command to run when schema application fails
- `-on-apply-succeeded`: Command to run after schema is successfully applied

### Example

```bash
docker run \
  -e S3_BUCKET=my-bucket \
  -e DB_HOST=localhost \
  -e DB_PORT=5432 \
  -e DB_USER=user \
  -e DB_PASSWORD=pass \
  -e DB_NAME=mydb \
  db-schema-sync \
    -path-prefix schemas/ \
    -schema-file schema.sql \
    -interval 30s \
    -on-apply-succeeded "curl -X POST https://my-api/notify" \
    -on-apply-failed "curl -X POST https://my-api/alert"
```
