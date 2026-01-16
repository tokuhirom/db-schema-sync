//go:build integration

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
)

func setupLocalStack(t *testing.T) (*s3.Client, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := localstack.Run(ctx, "localstack/localstack:latest")
	if err != nil {
		t.Fatalf("failed to start localstack: %v", err)
	}

	endpoint, err := container.PortEndpoint(ctx, "4566/tcp", "http")
	if err != nil {
		t.Fatalf("failed to get endpoint: %v", err)
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	cleanup := func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	}

	return client, cleanup
}

func createBucket(t *testing.T, ctx context.Context, client *s3.Client, bucket string) {
	t.Helper()
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		t.Fatalf("failed to create bucket: %v", err)
	}
}

func putObject(t *testing.T, ctx context.Context, client *s3.Client, bucket, key, content string) {
	t.Helper()
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(content),
	})
	if err != nil {
		t.Fatalf("failed to put object: %v", err)
	}
}

func TestFindLatestSchema(t *testing.T) {
	client, cleanup := setupLocalStack(t)
	defer cleanup()

	ctx := context.Background()
	bucket := "test-bucket"
	createBucket(t, ctx, client, bucket)

	// Setup test data: multiple versions
	putObject(t, ctx, client, bucket, "schemas/v1/schema.sql", "CREATE TABLE t1;")
	putObject(t, ctx, client, bucket, "schemas/v2/schema.sql", "CREATE TABLE t2;")
	putObject(t, ctx, client, bucket, "schemas/v3/schema.sql", "CREATE TABLE t3;")

	t.Run("finds latest version", func(t *testing.T) {
		key, version, err := findLatestSchema(ctx, client, bucket, "schemas/", "schema.sql")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if version != "v3" {
			t.Errorf("expected version v3, got %s", version)
		}
		if key != "schemas/v3/schema.sql" {
			t.Errorf("expected key schemas/v3/schema.sql, got %s", key)
		}
	})

	t.Run("returns error when no schema files", func(t *testing.T) {
		_, _, err := findLatestSchema(ctx, client, bucket, "nonexistent/", "schema.sql")
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestDownloadSchemaFromS3(t *testing.T) {
	client, cleanup := setupLocalStack(t)
	defer cleanup()

	ctx := context.Background()
	bucket := "test-bucket"
	createBucket(t, ctx, client, bucket)

	expectedContent := "CREATE TABLE users (id INT);"
	putObject(t, ctx, client, bucket, "schemas/v1/schema.sql", expectedContent)

	t.Run("downloads schema successfully", func(t *testing.T) {
		content, err := downloadSchemaFromS3(ctx, client, bucket, "schemas/v1/schema.sql")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(content) != expectedContent {
			t.Errorf("expected %q, got %q", expectedContent, string(content))
		}
	})

	t.Run("returns error for non-existent key", func(t *testing.T) {
		_, err := downloadSchemaFromS3(ctx, client, bucket, "nonexistent/schema.sql")
		if err == nil {
			t.Error("expected error, got nil")
		}
	})
}

func TestCheckCompletionMarker(t *testing.T) {
	client, cleanup := setupLocalStack(t)
	defer cleanup()

	ctx := context.Background()
	bucket := "test-bucket"
	createBucket(t, ctx, client, bucket)

	putObject(t, ctx, client, bucket, "schemas/v1/schema.sql", "CREATE TABLE t1;")
	putObject(t, ctx, client, bucket, "schemas/v1/completed", "")

	t.Run("returns true when marker exists", func(t *testing.T) {
		exists, err := checkCompletionMarker(ctx, client, bucket, "schemas/v1/schema.sql", "completed")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !exists {
			t.Error("expected marker to exist")
		}
	})

	t.Run("returns false when marker does not exist", func(t *testing.T) {
		exists, err := checkCompletionMarker(ctx, client, bucket, "schemas/v2/schema.sql", "completed")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exists {
			t.Error("expected marker to not exist")
		}
	})
}

func TestCreateCompletionMarker(t *testing.T) {
	client, cleanup := setupLocalStack(t)
	defer cleanup()

	ctx := context.Background()
	bucket := "test-bucket"
	createBucket(t, ctx, client, bucket)

	putObject(t, ctx, client, bucket, "schemas/v1/schema.sql", "CREATE TABLE t1;")

	t.Run("creates marker successfully", func(t *testing.T) {
		err := createCompletionMarker(ctx, client, bucket, "schemas/v1/schema.sql", "completed")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify marker exists
		exists, err := checkCompletionMarker(ctx, client, bucket, "schemas/v1/schema.sql", "completed")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !exists {
			t.Error("expected marker to exist after creation")
		}
	})
}
