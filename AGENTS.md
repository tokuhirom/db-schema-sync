# Agent Guide for DB Schema Sync

This document provides essential information for agents working with the DB Schema Sync codebase.

## Project Overview

DB Schema Sync is a Go application that continuously synchronizes database schemas from S3 using psqldef. It periodically polls S3 for schema file updates and automatically applies new schema versions when detected, similar to how dewy works for continuous delivery.

Schema files are expected to be organized in S3 with the following structure:
```
s3://bucket/path-prefix/version/schema.sql
```

Where:
- `path-prefix` is specified with the `-path-prefix` flag
- `version` is a timestamp or version identifier (sorted alphabetically)
- `schema.sql` is the schema file name specified with the `-schema-file` flag

## Code Organization

- `cmd/main.go` - Main application entry point
- `Dockerfile` - Multi-stage Docker image build
- `go.mod` - Go module dependencies
- `Makefile` - Build and development commands

## Essential Commands

### Build
```bash
# Using make
make build

# Or directly with go
go build -o db-schema-sync cmd/main.go
```

### Run
```bash
# Using make
make run

# Or directly with go
# Set required environment variables
export S3_BUCKET=your-bucket
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=user
export DB_PASSWORD=password
export DB_NAME=dbname

# Run with path prefix and default schema file name
./db-schema-sync -path-prefix schemas/

# Run with custom path prefix and schema file name
./db-schema-sync -path-prefix prod/schemas/ -schema-file database.sql -interval 30s

# Run with pre-apply and post-apply commands
./db-schema-sync -path-prefix schemas/ -pre-apply "echo 'Starting schema sync'" -post-apply "curl -X POST https://my-api/notify"
```

### Install/Update Dependencies
```bash
# Using make
make deps

# Or directly with go
go mod tidy
```

### Docker Build
```bash
# Using make
make docker-build

# Or directly with docker
docker build -t db-schema-sync .
```

### Docker Run
```bash
# Using make
make docker-run

# Or directly with docker
docker run -e S3_BUCKET=my-bucket -e DB_HOST=localhost -e DB_PORT=5432 -e DB_USER=user -e DB_PASSWORD=pass -e DB_NAME=mydb db-schema-sync -path-prefix schemas/ -interval 30s -post-apply "curl -X POST https://my-api/notify"
```

### Clean Build Artifacts
```bash
# Using make
make clean
```

## Dependencies

- Go 1.25.5
- AWS SDK for Go (v1.55.8)
- sqldef (specifically psqldef for PostgreSQL, v3.9.4)
- Docker (for containerization)

Note: The AWS SDK for Go v1 is deprecated. Consider migrating to AWS SDK for Go v2 in future updates.

## Code Patterns and Conventions

### Error Handling
- Uses Go's idiomatic error wrapping with `fmt.Errorf("message: %w", err)`
- Logs errors but continues operation for resilience
- Pre-allocates error messages with context at the point of failure

### Environment Variables
- All database connection parameters are read from environment variables
- S3 bucket name is also read from environment variables
- No default values for connection parameters - they must all be set
- AWS credentials are automatically used by the AWS SDK (standard AWS environment variables)

### Command Line Flags
- Uses Go's `flag` package for command line parsing
- Supported flags:
  - `-path-prefix`: S3 path prefix (e.g., "schemas/" or "prod/schemas/") - required
  - `-schema-file`: Schema file name within the path prefix (default: "schema.sql")
  - `-interval`: Polling interval as a duration (default: 1m0s)
  - `-pre-apply`: Command to run before applying schema (default: "")
  - `-post-apply`: Command to run after applying schema (default: "")
  - `-state-file`: File to store the last applied version (default: "/tmp/db-schema-sync-state")
  - `-completed-file`: File to indicate schema application completion (default: "/tmp/db-schema-sync-completed")

### AWS Integration
- Uses AWS SDK for Go to download files from S3
- Creates a new AWS session with default configuration
- Requires AWS credentials to be configured in the environment (standard AWS environment variables)

### Database Operations
- Uses psqldef command-line tool to apply schemas
- Generates temporary files to store downloaded schemas
- Passes database credentials directly to psqldef via command line arguments

### Continuous Operation
- Implements a polling loop with configurable interval
- Continues running indefinitely, checking for updates at regular intervals
- Logs each polling cycle for monitoring

### Version Management
- Finds all versions of schema files in the specified S3 path
- Sorts versions alphabetically and applies the latest
- Tracks last applied version in a state file to avoid re-applying the same schema
- Only applies schemas that are lexicographically greater than the last applied version
- Creates a completion marker file after successful schema application

## Testing Approach

Currently, there are no automated tests in the codebase. Testing would need to be done manually or through integration tests that involve:
- Setting up a test PostgreSQL database
- Uploading test schema files to S3 with versioned paths
- Running the application with test configurations
- Verifying that schema updates are properly detected and applied
- Confirming that the same version is not applied multiple times
- Checking that completion marker files are created

## Important Gotchas

1. **PostgreSQL Dependency**: The application requires psqldef to be installed, which is handled in the Dockerfile but needs to be manually installed for local development.

2. **Environment Variables**: All database connection variables must be set, or the application will fail with an error message.

3. **Path Prefix Requirement**: The `-path-prefix` flag is required and must end with a slash.

4. **Post-Apply Execution**: The post-apply command runs even if schema application fails, but pre-apply only runs once at startup.

5. **Temporary Files**: Schema files are temporarily written to disk during processing and then deleted.

6. **AWS Credentials**: The application relies on AWS SDK's default credential chain, which means credentials must be configured in the environment.

7. **AWS SDK Version**: The project uses AWS SDK for Go v1, which is deprecated. Future updates should consider migrating to AWS SDK for Go v2.

8. **Continuous Operation**: The application runs indefinitely in a loop, so it's designed for long-running deployment rather than one-time execution.

9. **Error Resilience**: The application logs errors but continues running, so monitoring logs is important for detecting issues.

10. **Interval Format**: The interval flag accepts duration strings (e.g., "30s", "1m30s", "2h") rather than just seconds.

11. **SQLDef Version**: The application uses psqldef v3.9.4, which is the latest version as of January 2026.

12. **Version Comparison**: Versions are compared lexicographically, so timestamps should be in a format that sorts correctly (e.g., YYYYMMDDHHMMSS).

13. **State Tracking**: The application tracks the last applied version in a state file to avoid re-applying the same schema.

14. **Completion Marker**: After successful schema application, an empty completion marker file is created to indicate that the process has completed.

## Deployment

The application is designed to be deployed as a Docker container. The Dockerfile handles:
- Building the Go application
- Installing psqldef v3.9.4
- Setting the entrypoint to the application

Deployment typically involves:
1. Building the Docker image
2. Configuring environment variables for database and S3 access
3. Running the container with appropriate flags
4. Monitoring logs for schema update events
5. Checking for completion marker files to verify successful schema applications