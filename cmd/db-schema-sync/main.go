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

// CLI defines command line options
type CLI struct {
	// S3 settings
	S3Bucket   string `help:"S3 bucket name" env:"S3_BUCKET" required:""`
	S3Endpoint string `help:"Custom S3 endpoint URL for S3-compatible storage (e.g., 'https://s3.isk01.sakurastorage.jp')" env:"S3_ENDPOINT"`
	PathPrefix string `help:"S3 path prefix (e.g., 'schemas/')" env:"PATH_PREFIX" required:""`
	SchemaFile string `help:"Schema file name" env:"SCHEMA_FILE" default:"schema.sql"`

	// Database settings
	DBHost     string `help:"Database host" env:"DB_HOST" required:""`
	DBPort     string `help:"Database port" env:"DB_PORT" required:""`
	DBUser     string `help:"Database user" env:"DB_USER" required:""`
	DBPassword string `help:"Database password" env:"DB_PASSWORD" required:""`
	DBName     string `help:"Database name" env:"DB_NAME" required:""`

	// Polling settings
	Interval      time.Duration `help:"Polling interval" env:"INTERVAL" default:"1m"`
	CompletedFile string        `help:"Completion marker file name" env:"COMPLETED_FILE" default:"completed"`

	// Lifecycle hooks
	OnStart          string `help:"Command to run when the process starts" env:"ON_START"`
	OnS3FetchError   string `help:"Command to run when S3 fetch fails 3 times consecutively" env:"ON_S3_FETCH_ERROR"`
	OnApplyFailed    string `help:"Command to run when schema application fails" env:"ON_APPLY_FAILED"`
	OnApplySucceeded string `help:"Command to run after schema is successfully applied" env:"ON_APPLY_SUCCEEDED"`
}

var (
	cli CLI

	// In-memory state
	lastAppliedVersion      string
	consecutiveFailureCount int
)

func main() {
	kong.Parse(&cli,
		kong.Name("db-schema-sync"),
		kong.Description("Synchronize database schemas from S3 using psqldef"),
	)

	// Ensure pathPrefix ends with a slash
	if cli.PathPrefix != "" && cli.PathPrefix[len(cli.PathPrefix)-1] != '/' {
		cli.PathPrefix += "/"
	}

	// Run on-start command if specified
	if cli.OnStart != "" {
		if err := runCommand(cli.OnStart); err != nil {
			slog.Warn("on-start command failed", "error", err)
		}
	}

	ctx := context.Background()

	// Start polling loop
	for {
		if err := run(ctx); err != nil {
			slog.Error("Error in run", "error", err)
		}

		slog.Info("Waiting before next poll", "interval", cli.Interval)
		time.Sleep(cli.Interval)
	}
}

const maxConsecutiveFailures = 3

func run(ctx context.Context) error {
	// Initialize S3 client
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	var client *s3.Client
	if cli.S3Endpoint != "" {
		// Use custom endpoint for S3-compatible storage
		client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cli.S3Endpoint)
		})
		slog.Info("Using custom S3 endpoint", "endpoint", cli.S3Endpoint)
	} else {
		client = s3.NewFromConfig(cfg)
	}

	return runWithClient(ctx, client, cli.S3Bucket)
}

func runWithClient(ctx context.Context, client S3Client, bucket string) error {
	slog.Info("Finding latest schema...")

	// Find the latest schema file
	latestSchemaKey, latestVersion, err := findLatestSchema(ctx, client, bucket, cli.PathPrefix, cli.SchemaFile)
	if err != nil {
		consecutiveFailureCount++
		slog.Error("Failed to find latest schema", "error", err, "consecutive_failures", consecutiveFailureCount)

		if consecutiveFailureCount >= maxConsecutiveFailures {
			runHook("on-s3-fetch-error", cli.OnS3FetchError)
		}
		return fmt.Errorf("failed to find latest schema: %w", err)
	}

	// Reset failure count on success
	consecutiveFailureCount = 0

	if lastAppliedVersion != "" && compareVersions(latestVersion, lastAppliedVersion) <= 0 {
		slog.Info("Latest version is not newer than last applied version, skipping", "latest", latestVersion, "last_applied", lastAppliedVersion)
		return nil
	}

	// Check if completion marker already exists in S3
	if cli.CompletedFile != "" {
		exists, err := checkCompletionMarker(ctx, client, bucket, latestSchemaKey, cli.CompletedFile)
		if err != nil {
			slog.Warn("Could not check completion marker", "error", err)
		} else if exists {
			slog.Info("Completion marker already exists for version, skipping", "version", latestVersion)
			lastAppliedVersion = latestVersion
			return nil
		}
	}

	// Download schema from S3
	schema, err := downloadSchemaFromS3(ctx, client, bucket, latestSchemaKey)
	if err != nil {
		consecutiveFailureCount++
		if consecutiveFailureCount >= maxConsecutiveFailures {
			runHook("on-s3-fetch-error", cli.OnS3FetchError)
		}
		return fmt.Errorf("failed to download schema: %w", err)
	}

	// Apply schema using psqldef
	if err := applySchema(schema); err != nil {
		runHook("on-apply-failed", cli.OnApplyFailed)
		return fmt.Errorf("failed to apply schema: %w", err)
	}

	// Record the applied version
	lastAppliedVersion = latestVersion

	// Create completion marker in S3
	if cli.CompletedFile != "" {
		if err := createCompletionMarker(ctx, client, bucket, latestSchemaKey, cli.CompletedFile); err != nil {
			slog.Warn("Could not create completion marker", "error", err)
		}
	}

	// Run on-apply-succeeded hook
	runHook("on-apply-succeeded", cli.OnApplySucceeded)

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

func applySchema(schema []byte) error {
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
	cmd := exec.Command("psqldef", "-U", cli.DBUser, "-h", cli.DBHost, "-p", cli.DBPort, "--password", cli.DBPassword, cli.DBName, "--file", tmpFile.Name())
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

func runCommand(command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
