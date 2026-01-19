package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/hashicorp/go-version"
)

// S3Client defines the interface for S3 operations
type S3Client interface {
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// CLI defines the command line interface with subcommands
type CLI struct {
	// Global S3 settings
	S3Bucket   string `name:"s3-bucket" help:"S3 bucket name" env:"S3_BUCKET" required:""`
	S3Endpoint string `name:"s3-endpoint" help:"Custom S3 endpoint URL for S3-compatible storage" env:"S3_ENDPOINT"`
	PathPrefix string `help:"S3 path prefix (e.g., 'schemas/')" env:"PATH_PREFIX" required:""`
	SchemaFile string `help:"Schema file name" env:"SCHEMA_FILE" default:"schema.sql"`

	// Completion marker
	CompletedFile string `help:"Completion marker file name" env:"COMPLETED_FILE" default:"completed"`

	// Subcommands
	Watch WatchCmd `cmd:"" help:"Run in daemon mode, continuously polling for schema updates"`
	Apply ApplyCmd `cmd:"" help:"Apply schema once and exit"`
	Diff  DiffCmd  `cmd:"" help:"Show diff between S3 schema and local file"`
}

// WatchCmd runs the sync in daemon mode with polling
type WatchCmd struct {
	// Database settings
	DBHost     string `help:"Database host" env:"DB_HOST" required:""`
	DBPort     string `help:"Database port" env:"DB_PORT" required:""`
	DBUser     string `help:"Database user" env:"DB_USER" required:""`
	DBPassword string `help:"Database password" env:"DB_PASSWORD" required:""`
	DBName     string `help:"Database name" env:"DB_NAME" required:""`

	// Polling settings
	Interval time.Duration `help:"Polling interval" env:"INTERVAL" default:"1m"`

	// Metrics settings
	MetricsAddr string `help:"Metrics endpoint address (e.g., ':9090'). Metrics disabled if not set" env:"METRICS_ADDR"`

	// Lifecycle hooks
	OnStart          string `help:"Command to run when the process starts" env:"ON_START"`
	OnS3FetchError   string `help:"Command to run when S3 fetch fails 3 times consecutively" env:"ON_S3_FETCH_ERROR"`
	OnBeforeApply    string `help:"Command to run before schema application starts" env:"ON_BEFORE_APPLY"`
	OnApplyFailed    string `help:"Command to run when schema application fails" env:"ON_APPLY_FAILED"`
	OnApplySucceeded string `help:"Command to run after schema is successfully applied" env:"ON_APPLY_SUCCEEDED"`
}

// ApplyCmd applies the schema once and exits
type ApplyCmd struct {
	// Database settings
	DBHost     string `help:"Database host" env:"DB_HOST" required:""`
	DBPort     string `help:"Database port" env:"DB_PORT" required:""`
	DBUser     string `help:"Database user" env:"DB_USER" required:""`
	DBPassword string `help:"Database password" env:"DB_PASSWORD" required:""`
	DBName     string `help:"Database name" env:"DB_NAME" required:""`

	// Lifecycle hooks
	OnBeforeApply    string `help:"Command to run before schema application starts" env:"ON_BEFORE_APPLY"`
	OnApplyFailed    string `help:"Command to run when schema application fails" env:"ON_APPLY_FAILED"`
	OnApplySucceeded string `help:"Command to run after schema is successfully applied" env:"ON_APPLY_SUCCEEDED"`
}

// DiffCmd shows the diff between S3 schema and a local file or database using psqldef
type DiffCmd struct {
	LocalFile  string `arg:"" optional:"" help:"Local schema file to compare against S3 (uses psqldef --dry-run for both)"`
	DBHost     string `help:"Database host" env:"DB_HOST" required:""`
	DBPort     string `help:"Database port" env:"DB_PORT" required:""`
	DBUser     string `help:"Database user" env:"DB_USER" required:""`
	DBPassword string `help:"Database password" env:"DB_PASSWORD" required:""`
	DBName     string `help:"Database name" env:"DB_NAME" required:""`
}

var (
	cli CLI

	// In-memory state (for watch mode)
	lastAppliedVersion      string
	consecutiveFailureCount int
)

const maxConsecutiveFailures = 3

func main() {
	ctx := kong.Parse(&cli,
		kong.Name("db-schema-sync"),
		kong.Description(`Synchronize database schemas from S3 using psqldef

AWS credentials can be configured via environment variables:
  AWS_ACCESS_KEY_ID       AWS access key ID
  AWS_SECRET_ACCESS_KEY   AWS secret access key
  AWS_SESSION_TOKEN       AWS session token (for temporary credentials)
  AWS_DEFAULT_REGION      AWS region (optional)

Or use IAM roles when running on EC2, ECS, or EKS.`),
		kong.UsageOnError(),
	)

	// Ensure pathPrefix ends with a slash
	if cli.PathPrefix != "" && cli.PathPrefix[len(cli.PathPrefix)-1] != '/' {
		cli.PathPrefix += "/"
	}

	err := ctx.Run(&cli)
	ctx.FatalIfErrorf(err)
}

// Run executes the watch command
func (cmd *WatchCmd) Run(cli *CLI) error {
	// Start metrics server if address is specified
	if cmd.MetricsAddr != "" {
		go startMetricsServer(cmd.MetricsAddr)
	}

	// Run on-start command if specified
	if cmd.OnStart != "" {
		if err := runCommand(cmd.OnStart); err != nil {
			slog.Warn("on-start command failed", "error", err)
		}
	}

	ctx := context.Background()
	client, err := createS3Client(ctx, cli.S3Endpoint)
	if err != nil {
		return err
	}

	// Start polling loop
	for {
		if err := runSync(ctx, client, cli, cmd.DBHost, cmd.DBPort, cmd.DBUser, cmd.DBPassword, cmd.DBName, cmd.OnS3FetchError, cmd.OnBeforeApply, cmd.OnApplyFailed, cmd.OnApplySucceeded); err != nil {
			slog.Error("Error in sync", "error", err)
		}

		slog.Info("Waiting before next poll", "interval", cmd.Interval)
		time.Sleep(cmd.Interval)
	}
}

// Run executes the apply command (single-shot)
func (cmd *ApplyCmd) Run(cli *CLI) error {
	ctx := context.Background()
	client, err := createS3Client(ctx, cli.S3Endpoint)
	if err != nil {
		return err
	}

	return runSync(ctx, client, cli, cmd.DBHost, cmd.DBPort, cmd.DBUser, cmd.DBPassword, cmd.DBName, "", cmd.OnBeforeApply, cmd.OnApplyFailed, cmd.OnApplySucceeded)
}

// Run executes the diff command
func (cmd *DiffCmd) Run(cli *CLI) error {
	ctx := context.Background()
	client, err := createS3Client(ctx, cli.S3Endpoint)
	if err != nil {
		return err
	}

	// Find the latest completed schema
	latestSchemaKey, latestVersion, err := findLatestCompletedSchema(ctx, client, cli.S3Bucket, cli.PathPrefix, cli.SchemaFile, cli.CompletedFile)
	if err != nil {
		return fmt.Errorf("failed to find latest completed schema: %w", err)
	}

	slog.Info("Found latest completed schema", "version", latestVersion, "key", latestSchemaKey)

	// Download schema from S3
	s3Schema, err := downloadSchemaFromS3(ctx, client, cli.S3Bucket, latestSchemaKey)
	if err != nil {
		return fmt.Errorf("failed to download schema from S3: %w", err)
	}

	// Show diff using psqldef --dry-run
	if cmd.LocalFile != "" {
		// Compare local file vs S3 schema against DB
		return showFileDiff(s3Schema, cmd.LocalFile, cmd.DBHost, cmd.DBPort, cmd.DBUser, cmd.DBPassword, cmd.DBName)
	}
	// Compare S3 schema against DB
	return showSchemaDiff(s3Schema, cmd.DBHost, cmd.DBPort, cmd.DBUser, cmd.DBPassword, cmd.DBName)
}

func createS3Client(ctx context.Context, endpoint string) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	if endpoint != "" {
		client := s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
		slog.Info("Using custom S3 endpoint", "endpoint", endpoint)
		return client, nil
	}

	return s3.NewFromConfig(cfg), nil
}

func runSync(ctx context.Context, client S3Client, cli *CLI, dbHost, dbPort, dbUser, dbPassword, dbName, onS3FetchError, onBeforeApply, onApplyFailed, onApplySucceeded string) error {
	slog.Info("Finding latest schema...")

	// Record S3 fetch attempt
	recordS3FetchAttempt()

	// Find the latest schema file
	latestSchemaKey, latestVersion, err := findLatestSchema(ctx, client, cli.S3Bucket, cli.PathPrefix, cli.SchemaFile)
	if err != nil {
		consecutiveFailureCount++
		recordS3FetchError()
		recordConsecutiveFailures(consecutiveFailureCount)
		slog.Error("Failed to find latest schema", "error", err, "consecutive_failures", consecutiveFailureCount)

		if consecutiveFailureCount >= maxConsecutiveFailures {
			runHook("on-s3-fetch-error", onS3FetchError)
		}
		return fmt.Errorf("failed to find latest schema: %w", err)
	}

	// Reset failure count on success
	consecutiveFailureCount = 0
	recordConsecutiveFailures(consecutiveFailureCount)

	if lastAppliedVersion != "" && compareVersions(latestVersion, lastAppliedVersion) <= 0 {
		slog.Info("Latest version is not newer than last applied version, skipping", "latest", latestVersion, "last_applied", lastAppliedVersion)
		return nil
	}

	// Check if completion marker already exists in S3
	if cli.CompletedFile != "" {
		exists, err := checkCompletionMarker(ctx, client, cli.S3Bucket, latestSchemaKey, cli.CompletedFile)
		if err != nil {
			slog.Warn("Could not check completion marker", "error", err)
		} else if exists {
			slog.Info("Completion marker already exists for version, skipping", "version", latestVersion)
			lastAppliedVersion = latestVersion
			return nil
		}
	}

	// Download schema from S3
	schema, err := downloadSchemaFromS3(ctx, client, cli.S3Bucket, latestSchemaKey)
	if err != nil {
		consecutiveFailureCount++
		recordS3FetchError()
		recordConsecutiveFailures(consecutiveFailureCount)
		if consecutiveFailureCount >= maxConsecutiveFailures {
			runHook("on-s3-fetch-error", onS3FetchError)
		}
		return fmt.Errorf("failed to download schema: %w", err)
	}

	// Run on-before-apply hook
	runHook("on-before-apply", onBeforeApply)

	// Record apply attempt
	recordApplyAttempt()

	// Apply schema using psqldef
	if err := applySchema(schema, dbHost, dbPort, dbUser, dbPassword, dbName); err != nil {
		recordApplyError()
		runHook("on-apply-failed", onApplyFailed)
		return fmt.Errorf("failed to apply schema: %w", err)
	}

	// Record successful apply
	recordApplySuccess(latestVersion)

	// Record the applied version
	lastAppliedVersion = latestVersion

	// Create completion marker in S3
	if cli.CompletedFile != "" {
		if err := createCompletionMarker(ctx, client, cli.S3Bucket, latestSchemaKey, cli.CompletedFile); err != nil {
			slog.Warn("Could not create completion marker", "error", err)
		}
	}

	// Run on-apply-succeeded hook
	runHook("on-apply-succeeded", onApplySucceeded)

	slog.Info("Successfully applied schema", "version", latestVersion)
	return nil
}

func runHook(name, command string) {
	if command == "" {
		return
	}
	slog.Info("Running hook", "hook", name)
	if err := runCommand(command); err != nil {
		slog.Error("Hook command failed", "hook", name, "error", err)
	}
}

func findLatestSchema(ctx context.Context, client S3Client, bucket, prefix, schemaFileName string) (string, string, error) {
	// List objects with the specified prefix
	resp, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return "", "", err
	}

	// Extract keys from response
	var keys []string
	for _, obj := range resp.Contents {
		keys = append(keys, *obj.Key)
	}

	return findLatestVersion(keys, prefix, schemaFileName)
}

// findLatestCompletedSchema finds the latest schema that has a completion marker
func findLatestCompletedSchema(ctx context.Context, client S3Client, bucket, prefix, schemaFileName, completedFileName string) (string, string, error) {
	// List objects with the specified prefix
	resp, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return "", "", err
	}

	// Build a set of keys for quick lookup
	keySet := make(map[string]bool)
	for _, obj := range resp.Contents {
		keySet[*obj.Key] = true
	}

	// Extract versions that have both schema file and completion marker
	var versionStrings []string
	for _, obj := range resp.Contents {
		key := *obj.Key
		if path.Base(key) == schemaFileName {
			// Check if completion marker exists
			markerKey := buildCompletionMarkerKey(key, completedFileName)
			if keySet[markerKey] {
				dir := path.Dir(key)
				ver := path.Base(dir)
				if ver != "." && ver != "/" {
					versionStrings = append(versionStrings, ver)
				}
			}
		}
	}

	if len(versionStrings) == 0 {
		return "", "", fmt.Errorf("no completed schema files found with prefix %s", prefix)
	}

	// Find the latest version
	latestVersion, err := findMaxVersion(versionStrings)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse versions: %w", err)
	}

	latestSchemaKey := path.Join(prefix, latestVersion, schemaFileName)
	return latestSchemaKey, latestVersion, nil
}

// findLatestVersion extracts versions from S3 keys and returns the latest one
func findLatestVersion(keys []string, prefix, schemaFileName string) (string, string, error) {
	var versionStrings []string
	for _, key := range keys {
		// Check if the object key ends with the schema file name
		if path.Base(key) == schemaFileName {
			// Extract the version part (directory name)
			dir := path.Dir(key)
			ver := path.Base(dir)
			// Only consider non-empty versions
			if ver != "." && ver != "/" {
				versionStrings = append(versionStrings, ver)
			}
		}
	}

	if len(versionStrings) == 0 {
		return "", "", fmt.Errorf("no schema files found with prefix %s and file name %s", prefix, schemaFileName)
	}

	// Sort versions using semantic versioning
	latestVersion, err := findMaxVersion(versionStrings)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse versions: %w", err)
	}

	// Construct the full key for the latest schema
	latestSchemaKey := path.Join(prefix, latestVersion, schemaFileName)
	return latestSchemaKey, latestVersion, nil
}

// findMaxVersion finds the maximum version from a list of version strings
func findMaxVersion(versionStrings []string) (string, error) {
	if len(versionStrings) == 0 {
		return "", fmt.Errorf("no versions provided")
	}

	type versionPair struct {
		original string
		parsed   *version.Version
	}

	var versions []versionPair
	for _, vs := range versionStrings {
		v, err := version.NewVersion(vs)
		if err != nil {
			// If parsing fails, log warning and skip
			slog.Warn("Failed to parse version, skipping", "version", vs, "error", err)
			continue
		}
		versions = append(versions, versionPair{original: vs, parsed: v})
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no valid versions found")
	}

	// Sort by parsed version
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].parsed.LessThan(versions[j].parsed)
	})

	return versions[len(versions)-1].original, nil
}

// compareVersions compares two version strings and returns:
// -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	ver1, err1 := version.NewVersion(v1)
	ver2, err2 := version.NewVersion(v2)

	// If either version fails to parse, fall back to string comparison
	if err1 != nil || err2 != nil {
		if v1 < v2 {
			return -1
		} else if v1 > v2 {
			return 1
		}
		return 0
	}

	if ver1.LessThan(ver2) {
		return -1
	} else if ver1.GreaterThan(ver2) {
		return 1
	}
	return 0
}

func downloadSchemaFromS3(ctx context.Context, client S3Client, bucket, key string) ([]byte, error) {
	result, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = result.Body.Close() }()

	return io.ReadAll(result.Body)
}

func applySchema(schema []byte, dbHost, dbPort, dbUser, dbPassword, dbName string) error {
	// Save schema to temporary file
	tmpFile, err := os.CreateTemp("", "schema-*.sql")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	defer func() { _ = tmpFile.Close() }()

	if _, err := tmpFile.Write(schema); err != nil {
		return err
	}

	// Run psqldef to apply schema
	cmd := exec.Command("psqldef", "-U", dbUser, "-h", dbHost, "-p", dbPort, "--password", dbPassword, dbName, "--file", tmpFile.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// buildCompletionMarkerKey constructs the S3 key for the completion marker
func buildCompletionMarkerKey(schemaKey, completedFileName string) string {
	schemaDir := path.Dir(schemaKey)
	return path.Join(schemaDir, completedFileName)
}

func checkCompletionMarker(ctx context.Context, client S3Client, bucket, schemaKey, completedFileName string) (bool, error) {
	markerKey := buildCompletionMarkerKey(schemaKey, completedFileName)

	// Check if the object exists
	_, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(markerKey),
	})

	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "NotFound") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func createCompletionMarker(ctx context.Context, client S3Client, bucket, schemaKey, completedFileName string) error {
	markerKey := buildCompletionMarkerKey(schemaKey, completedFileName)

	// Upload an empty file as the completion marker
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(markerKey),
		Body:   strings.NewReader(""),
	})

	return err
}

// showSchemaDiff compares S3 schema against actual database using psqldef --dry-run
func showSchemaDiff(s3Schema []byte, dbHost, dbPort, dbUser, dbPassword, dbName string) error {
	// Save schema to temporary file
	tmpFile, err := os.CreateTemp("", "schema-*.sql")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	defer func() { _ = tmpFile.Close() }()

	if _, err := tmpFile.Write(s3Schema); err != nil {
		return err
	}

	// Run psqldef with --dry-run to show what would be applied
	cmd := exec.Command("psqldef", "-U", dbUser, "-h", dbHost, "-p", dbPort, "--password", dbPassword, dbName, "--dry-run", "--file", tmpFile.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// showFileDiff compares local file vs S3 schema by running psqldef --dry-run on both and showing the difference
func showFileDiff(s3Schema []byte, localPath, dbHost, dbPort, dbUser, dbPassword, dbName string) error {
	// Get DDL diff for S3 schema
	s3DDL, err := getDryRunOutput(s3Schema, dbHost, dbPort, dbUser, dbPassword, dbName)
	if err != nil {
		return fmt.Errorf("failed to get dry-run for S3 schema: %w", err)
	}

	// Read local file
	localSchema, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read local file: %w", err)
	}

	// Get DDL diff for local schema
	localDDL, err := getDryRunOutput(localSchema, dbHost, dbPort, dbUser, dbPassword, dbName)
	if err != nil {
		return fmt.Errorf("failed to get dry-run for local schema: %w", err)
	}

	// Compare the two DDL outputs
	if s3DDL == localDDL {
		fmt.Println("No differences found between S3 schema and local file.")
		return nil
	}

	// Create temp files for diff
	s3TmpFile, err := os.CreateTemp("", "s3-ddl-*.sql")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(s3TmpFile.Name()) }()

	localTmpFile, err := os.CreateTemp("", "local-ddl-*.sql")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(localTmpFile.Name()) }()

	if _, err := s3TmpFile.WriteString(s3DDL); err != nil {
		return err
	}
	if err := s3TmpFile.Close(); err != nil {
		return err
	}

	if _, err := localTmpFile.WriteString(localDDL); err != nil {
		return err
	}
	if err := localTmpFile.Close(); err != nil {
		return err
	}

	// Run diff command
	cmd := exec.Command("diff", "-u", "--label", "S3 schema DDL", "--label", "Local file DDL", s3TmpFile.Name(), localTmpFile.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		// diff returns exit code 1 if files differ, which is not an error for us
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		return err
	}

	return nil
}

// getDryRunOutput runs psqldef --dry-run and captures the output
func getDryRunOutput(schema []byte, dbHost, dbPort, dbUser, dbPassword, dbName string) (string, error) {
	// Save schema to temporary file
	tmpFile, err := os.CreateTemp("", "schema-*.sql")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	defer func() { _ = tmpFile.Close() }()

	if _, err := tmpFile.Write(schema); err != nil {
		return "", err
	}

	// Run psqldef with --dry-run
	cmd := exec.Command("psqldef", "-U", dbUser, "-h", dbHost, "-p", dbPort, "--password", dbPassword, dbName, "--dry-run", "--file", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("psqldef --dry-run failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

func runCommand(command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
