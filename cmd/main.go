package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Client defines the interface for S3 operations
type S3Client interface {
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

var (
	pathPrefix    = flag.String("path-prefix", "", "S3 path prefix (e.g., 'schemas/' or 'prod/schemas/')")
	schemaFile    = flag.String("schema-file", "schema.sql", "Schema file name within the path prefix")
	interval      = flag.Duration("interval", 60*time.Second, "Polling interval")
	completedFile = flag.String("completed-file", "completed", "Name of completion marker file to create alongside schema file (default: 'completed')")

	// Lifecycle hooks
	onStart          = flag.String("on-start", "", "Command to run when the process starts")
	onS3FetchError   = flag.String("on-s3-fetch-error", "", "Command to run when S3 fetch fails 3 times consecutively")
	onApplyFailed    = flag.String("on-apply-failed", "", "Command to run when schema application fails")
	onApplySucceeded = flag.String("on-apply-succeeded", "", "Command to run after schema is successfully applied")

	// In-memory state
	lastAppliedVersion      string
	consecutiveFailureCount int
)

func main() {
	flag.Parse()

	if *pathPrefix == "" {
		slog.Error("path-prefix is required")
		os.Exit(1)
	}

	// Ensure pathPrefix ends with a slash
	if (*pathPrefix)[len(*pathPrefix)-1] != '/' {
		*pathPrefix += "/"
	}

	// Run on-start command if specified
	if *onStart != "" {
		if err := runCommand(*onStart); err != nil {
			slog.Warn("on-start command failed", "error", err)
		}
	}

	ctx := context.Background()

	// Start polling loop
	for {
		if err := run(ctx); err != nil {
			slog.Error("Error in run", "error", err)
		}

		slog.Info("Waiting before next poll", "interval", *interval)
		time.Sleep(*interval)
	}
}

const maxConsecutiveFailures = 3

func run(ctx context.Context) error {
	// Initialize S3 client
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}
	client := s3.NewFromConfig(cfg)

	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		return fmt.Errorf("S3_BUCKET environment variable not set")
	}

	return runWithClient(ctx, client, bucket)
}

func runWithClient(ctx context.Context, client S3Client, bucket string) error {
	slog.Info("Finding latest schema...")

	// Find the latest schema file
	latestSchemaKey, latestVersion, err := findLatestSchema(ctx, client, bucket, *pathPrefix, *schemaFile)
	if err != nil {
		consecutiveFailureCount++
		slog.Error("Failed to find latest schema", "error", err, "consecutive_failures", consecutiveFailureCount)

		if consecutiveFailureCount >= maxConsecutiveFailures {
			runHook("on-s3-fetch-error", *onS3FetchError)
		}
		return fmt.Errorf("failed to find latest schema: %w", err)
	}

	// Reset failure count on success
	consecutiveFailureCount = 0

	if lastAppliedVersion != "" && latestVersion <= lastAppliedVersion {
		slog.Info("Latest version is not newer than last applied version, skipping", "latest", latestVersion, "last_applied", lastAppliedVersion)
		return nil
	}

	// Check if completion marker already exists in S3
	if *completedFile != "" {
		exists, err := checkCompletionMarker(ctx, client, bucket, latestSchemaKey, *completedFile)
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
			runHook("on-s3-fetch-error", *onS3FetchError)
		}
		return fmt.Errorf("failed to download schema: %w", err)
	}

	// Apply schema using psqldef
	if err := applySchema(schema); err != nil {
		runHook("on-apply-failed", *onApplyFailed)
		return fmt.Errorf("failed to apply schema: %w", err)
	}

	// Record the applied version
	lastAppliedVersion = latestVersion

	// Create completion marker in S3
	if *completedFile != "" {
		if err := createCompletionMarker(ctx, client, bucket, latestSchemaKey, *completedFile); err != nil {
			slog.Warn("Could not create completion marker", "error", err)
		}
	}

	// Run on-apply-succeeded hook
	runHook("on-apply-succeeded", *onApplySucceeded)

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
	var versions []string
	for _, key := range keys {
		// Check if the object key ends with the schema file name
		if path.Base(key) == schemaFileName {
			// Extract the version part (directory name)
			dir := path.Dir(key)
			version := path.Base(dir)
			// Only consider non-empty versions
			if version != "." && version != "/" {
				versions = append(versions, version)
			}
		}
	}

	if len(versions) == 0 {
		return "", "", fmt.Errorf("no schema files found with prefix %s and file name %s", prefix, schemaFileName)
	}

	// Sort versions alphabetically and pick the latest
	sort.Strings(versions)
	latestVersion := versions[len(versions)-1]

	// Construct the full key for the latest schema
	latestSchemaKey := path.Join(prefix, latestVersion, schemaFileName)
	return latestSchemaKey, latestVersion, nil
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

	// Get database connection details from environment variables
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	database := os.Getenv("DB_NAME")

	if host == "" || port == "" || user == "" || password == "" || database == "" {
		return fmt.Errorf("database connection environment variables not set")
	}

	// Run psqldef to apply schema
	cmd := exec.Command("psqldef", "-U", user, "-h", host, "-p", port, "--password", password, database, "--file", tmpFile.Name())
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
