//go:build !integration

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// mockS3Client implements S3Client interface for testing
type mockS3Client struct {
	listObjectsFunc func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	getObjectFunc   func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	headObjectFunc  func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	putObjectFunc   func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

func (m *mockS3Client) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.listObjectsFunc != nil {
		return m.listObjectsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getObjectFunc != nil {
		return m.getObjectFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if m.headObjectFunc != nil {
		return m.headObjectFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putObjectFunc != nil {
		return m.putObjectFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("not implemented")
}

// Ensure mockS3Client implements S3Client interface
var _ S3Client = (*mockS3Client)(nil)

func TestFindLatestVersion(t *testing.T) {
	tests := []struct {
		name           string
		keys           []string
		prefix         string
		schemaFileName string
		wantKey        string
		wantVersion    string
		wantErr        bool
	}{
		{
			name: "finds latest version from multiple versions",
			keys: []string{
				"schemas/v1/schema.sql",
				"schemas/v2/schema.sql",
				"schemas/v3/schema.sql",
			},
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			wantKey:        "schemas/v3/schema.sql",
			wantVersion:    "v3",
			wantErr:        false,
		},
		{
			name: "handles v10 correctly (semver sorting)",
			keys: []string{
				"schemas/v1/schema.sql",
				"schemas/v2/schema.sql",
				"schemas/v10/schema.sql",
				"schemas/v9/schema.sql",
			},
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			wantKey:        "schemas/v10/schema.sql",
			wantVersion:    "v10",
			wantErr:        false,
		},
		{
			name: "handles full semver versions",
			keys: []string{
				"schemas/1.0.0/schema.sql",
				"schemas/1.1.0/schema.sql",
				"schemas/2.0.0/schema.sql",
				"schemas/1.10.0/schema.sql",
			},
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			wantKey:        "schemas/2.0.0/schema.sql",
			wantVersion:    "2.0.0",
			wantErr:        false,
		},
		{
			name: "handles semver with v prefix",
			keys: []string{
				"schemas/v1.0.0/schema.sql",
				"schemas/v1.1.0/schema.sql",
				"schemas/v2.0.0/schema.sql",
				"schemas/v1.10.0/schema.sql",
			},
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			wantKey:        "schemas/v2.0.0/schema.sql",
			wantVersion:    "v2.0.0",
			wantErr:        false,
		},
		{
			name: "handles patch versions correctly",
			keys: []string{
				"schemas/v1.0.0/schema.sql",
				"schemas/v1.0.1/schema.sql",
				"schemas/v1.0.10/schema.sql",
				"schemas/v1.0.2/schema.sql",
			},
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			wantKey:        "schemas/v1.0.10/schema.sql",
			wantVersion:    "v1.0.10",
			wantErr:        false,
		},
		{
			name: "handles timestamp versions",
			keys: []string{
				"schemas/20240101120000/schema.sql",
				"schemas/20240102120000/schema.sql",
				"schemas/20240103120000/schema.sql",
			},
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			wantKey:        "schemas/20240103120000/schema.sql",
			wantVersion:    "20240103120000",
			wantErr:        false,
		},
		{
			name: "ignores non-matching files",
			keys: []string{
				"schemas/v1/schema.sql",
				"schemas/v2/other.sql",
				"schemas/v3/schema.sql",
			},
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			wantKey:        "schemas/v3/schema.sql",
			wantVersion:    "v3",
			wantErr:        false,
		},
		{
			name:           "returns error when no schema files found",
			keys:           []string{},
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			wantErr:        true,
		},
		{
			name: "returns error when no matching schema files",
			keys: []string{
				"schemas/v1/other.sql",
				"schemas/v2/other.sql",
			},
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			wantErr:        true,
		},
		{
			name: "handles single version",
			keys: []string{
				"prod/schemas/v1/schema.sql",
			},
			prefix:         "prod/schemas/",
			schemaFileName: "schema.sql",
			wantKey:        "prod/schemas/v1/schema.sql",
			wantVersion:    "v1",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey, gotVersion, err := findLatestVersion(tt.keys, tt.prefix, tt.schemaFileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("findLatestVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotKey != tt.wantKey {
					t.Errorf("findLatestVersion() gotKey = %v, want %v", gotKey, tt.wantKey)
				}
				if gotVersion != tt.wantVersion {
					t.Errorf("findLatestVersion() gotVersion = %v, want %v", gotVersion, tt.wantVersion)
				}
			}
		})
	}
}

func TestFindMaxVersion(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		want     string
		wantErr  bool
	}{
		{
			name:     "simple versions",
			versions: []string{"v1", "v2", "v3"},
			want:     "v3",
			wantErr:  false,
		},
		{
			name:     "v10 is greater than v9",
			versions: []string{"v1", "v9", "v10", "v2"},
			want:     "v10",
			wantErr:  false,
		},
		{
			name:     "semver without v prefix",
			versions: []string{"1.0.0", "1.1.0", "2.0.0", "1.10.0"},
			want:     "2.0.0",
			wantErr:  false,
		},
		{
			name:     "semver with v prefix",
			versions: []string{"v1.0.0", "v1.1.0", "v2.0.0", "v1.10.0"},
			want:     "v2.0.0",
			wantErr:  false,
		},
		{
			name:     "patch versions",
			versions: []string{"v1.0.1", "v1.0.10", "v1.0.2", "v1.0.9"},
			want:     "v1.0.10",
			wantErr:  false,
		},
		{
			name:     "mixed major versions",
			versions: []string{"v1.9.9", "v2.0.0", "v1.10.0"},
			want:     "v2.0.0",
			wantErr:  false,
		},
		{
			name:     "timestamp format YYYYMMDDHHMMSS",
			versions: []string{"20240101120000", "20240115093000", "20240102120000"},
			want:     "20240115093000",
			wantErr:  false,
		},
		{
			name:     "timestamp format with different days",
			versions: []string{"20240101000000", "20240131235959", "20240115120000"},
			want:     "20240131235959",
			wantErr:  false,
		},
		{
			name:     "empty list",
			versions: []string{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findMaxVersion(tt.versions)
			if (err != nil) != tt.wantErr {
				t.Errorf("findMaxVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("findMaxVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		{
			name: "v1 < v2",
			v1:   "v1",
			v2:   "v2",
			want: -1,
		},
		{
			name: "v2 > v1",
			v1:   "v2",
			v2:   "v1",
			want: 1,
		},
		{
			name: "v1 == v1",
			v1:   "v1",
			v2:   "v1",
			want: 0,
		},
		{
			name: "v9 < v10",
			v1:   "v9",
			v2:   "v10",
			want: -1,
		},
		{
			name: "v10 > v9",
			v1:   "v10",
			v2:   "v9",
			want: 1,
		},
		{
			name: "1.0.0 < 2.0.0",
			v1:   "1.0.0",
			v2:   "2.0.0",
			want: -1,
		},
		{
			name: "1.9.0 < 1.10.0",
			v1:   "1.9.0",
			v2:   "1.10.0",
			want: -1,
		},
		{
			name: "1.0.9 < 1.0.10",
			v1:   "1.0.9",
			v2:   "1.0.10",
			want: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %v, want %v", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestBuildCompletionMarkerKey(t *testing.T) {
	tests := []struct {
		name              string
		schemaKey         string
		completedFileName string
		want              string
	}{
		{
			name:              "builds marker key",
			schemaKey:         "schemas/v1/schema.sql",
			completedFileName: "completed",
			want:              "schemas/v1/completed",
		},
		{
			name:              "handles nested path",
			schemaKey:         "prod/schemas/20240101/schema.sql",
			completedFileName: ".done",
			want:              "prod/schemas/20240101/.done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCompletionMarkerKey(tt.schemaKey, tt.completedFileName)
			if got != tt.want {
				t.Errorf("buildCompletionMarkerKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHookEnvToEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		hookEnv  HookEnv
		expected map[string]string
	}{
		{
			name: "all fields populated",
			hookEnv: HookEnv{
				S3Bucket:      "my-bucket",
				PathPrefix:    "schemas/",
				SchemaFile:    "schema.sql",
				Version:       "v1.2.3",
				Error:         "connection refused",
				CompletedFile: "completed",
				AppVersion:    "v0.0.8",
			},
			expected: map[string]string{
				"DB_SCHEMA_SYNC_S3_BUCKET":      "my-bucket",
				"DB_SCHEMA_SYNC_PATH_PREFIX":    "schemas/",
				"DB_SCHEMA_SYNC_SCHEMA_FILE":    "schema.sql",
				"DB_SCHEMA_SYNC_VERSION":        "v1.2.3",
				"DB_SCHEMA_SYNC_ERROR":          "connection refused",
				"DB_SCHEMA_SYNC_COMPLETED_FILE": "completed",
				"DB_SCHEMA_SYNC_APP_VERSION":    "v0.0.8",
			},
		},
		{
			name: "only S3 fields",
			hookEnv: HookEnv{
				S3Bucket:   "test-bucket",
				PathPrefix: "test/",
				SchemaFile: "test.sql",
			},
			expected: map[string]string{
				"DB_SCHEMA_SYNC_S3_BUCKET":   "test-bucket",
				"DB_SCHEMA_SYNC_PATH_PREFIX": "test/",
				"DB_SCHEMA_SYNC_SCHEMA_FILE": "test.sql",
			},
		},
		{
			name: "with error message containing special characters",
			hookEnv: HookEnv{
				S3Bucket: "bucket",
				Error:    "error: \"connection\" failed with code=123",
			},
			expected: map[string]string{
				"DB_SCHEMA_SYNC_S3_BUCKET": "bucket",
				"DB_SCHEMA_SYNC_ERROR":     "error: \"connection\" failed with code=123",
			},
		},
		{
			name:     "empty hook env",
			hookEnv:  HookEnv{},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVars := tt.hookEnv.toEnvVars()

			// Create a map for easy lookup
			envMap := make(map[string]string)
			for _, env := range envVars {
				parts := strings.SplitN(env, "=", 2)
				if len(parts) == 2 {
					envMap[parts[0]] = parts[1]
				}
			}

			// Check expected variables are present
			for key, expectedValue := range tt.expected {
				if gotValue, ok := envMap[key]; !ok {
					t.Errorf("expected env var %s not found", key)
				} else if gotValue != expectedValue {
					t.Errorf("env var %s = %q, want %q", key, gotValue, expectedValue)
				}
			}

			// Check unexpected DB_SCHEMA_SYNC_ variables are not present
			for key := range envMap {
				if strings.HasPrefix(key, "DB_SCHEMA_SYNC_") {
					if _, ok := tt.expected[key]; !ok {
						t.Errorf("unexpected env var %s found", key)
					}
				}
			}
		})
	}
}

func TestHookEnvPreservesExistingEnv(t *testing.T) {
	hookEnv := &HookEnv{
		S3Bucket: "test-bucket",
		Version:  "v1.0.0",
	}

	envVars := hookEnv.toEnvVars()

	// Check that existing environment variables are preserved (like PATH, HOME, etc.)
	hasPath := false
	for _, env := range envVars {
		if strings.HasPrefix(env, "PATH=") {
			hasPath = true
			break
		}
	}

	if !hasPath {
		t.Error("expected PATH environment variable to be preserved")
	}
}

func TestBuildExportedSchemaKey(t *testing.T) {
	tests := []struct {
		name      string
		schemaKey string
		want      string
	}{
		{
			name:      "basic schema key",
			schemaKey: "schemas/v1/schema.sql",
			want:      "schemas/v1/exported.sql",
		},
		{
			name:      "nested path",
			schemaKey: "prod/schemas/v2.0.0/schema.sql",
			want:      "prod/schemas/v2.0.0/exported.sql",
		},
		{
			name:      "timestamp version",
			schemaKey: "db/20240101120000/schema.sql",
			want:      "db/20240101120000/exported.sql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildExportedSchemaKey(tt.schemaKey)
			if got != tt.want {
				t.Errorf("buildExportedSchemaKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDownloadSchemaFromS3(t *testing.T) {
	tests := []struct {
		name        string
		bucket      string
		key         string
		mockContent string
		mockErr     error
		wantContent string
		wantErr     bool
	}{
		{
			name:        "successful download",
			bucket:      "test-bucket",
			key:         "schemas/v1/schema.sql",
			mockContent: "CREATE TABLE users (id INT);",
			wantContent: "CREATE TABLE users (id INT);",
			wantErr:     false,
		},
		{
			name:    "download error",
			bucket:  "test-bucket",
			key:     "schemas/v1/schema.sql",
			mockErr: fmt.Errorf("access denied"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockS3Client{
				getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
					if tt.mockErr != nil {
						return nil, tt.mockErr
					}
					return &s3.GetObjectOutput{
						Body: io.NopCloser(strings.NewReader(tt.mockContent)),
					}, nil
				},
			}

			got, err := downloadSchemaFromS3(context.Background(), mock, tt.bucket, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("downloadSchemaFromS3() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && string(got) != tt.wantContent {
				t.Errorf("downloadSchemaFromS3() = %v, want %v", string(got), tt.wantContent)
			}
		})
	}
}

func TestCheckCompletionMarker(t *testing.T) {
	tests := []struct {
		name              string
		bucket            string
		schemaKey         string
		completedFileName string
		headErr           error
		wantExists        bool
		wantErr           bool
	}{
		{
			name:              "marker exists",
			bucket:            "test-bucket",
			schemaKey:         "schemas/v1/schema.sql",
			completedFileName: "completed",
			headErr:           nil,
			wantExists:        true,
			wantErr:           false,
		},
		{
			name:              "marker not found",
			bucket:            "test-bucket",
			schemaKey:         "schemas/v1/schema.sql",
			completedFileName: "completed",
			headErr:           fmt.Errorf("NotFound: The specified key does not exist"),
			wantExists:        false,
			wantErr:           false,
		},
		{
			name:              "other error",
			bucket:            "test-bucket",
			schemaKey:         "schemas/v1/schema.sql",
			completedFileName: "completed",
			headErr:           fmt.Errorf("access denied"),
			wantExists:        false,
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockS3Client{
				headObjectFunc: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
					if tt.headErr != nil {
						return nil, tt.headErr
					}
					return &s3.HeadObjectOutput{}, nil
				},
			}

			exists, err := checkCompletionMarker(context.Background(), mock, tt.bucket, tt.schemaKey, tt.completedFileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkCompletionMarker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if exists != tt.wantExists {
				t.Errorf("checkCompletionMarker() = %v, want %v", exists, tt.wantExists)
			}
		})
	}
}

func TestCreateCompletionMarker(t *testing.T) {
	tests := []struct {
		name              string
		bucket            string
		schemaKey         string
		completedFileName string
		putErr            error
		wantKey           string
		wantErr           bool
	}{
		{
			name:              "successful creation",
			bucket:            "test-bucket",
			schemaKey:         "schemas/v1/schema.sql",
			completedFileName: "completed",
			putErr:            nil,
			wantKey:           "schemas/v1/completed",
			wantErr:           false,
		},
		{
			name:              "put error",
			bucket:            "test-bucket",
			schemaKey:         "schemas/v1/schema.sql",
			completedFileName: "completed",
			putErr:            fmt.Errorf("access denied"),
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotKey string
			mock := &mockS3Client{
				putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
					if tt.putErr != nil {
						return nil, tt.putErr
					}
					gotKey = *params.Key
					return &s3.PutObjectOutput{}, nil
				},
			}

			err := createCompletionMarker(context.Background(), mock, tt.bucket, tt.schemaKey, tt.completedFileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("createCompletionMarker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && gotKey != tt.wantKey {
				t.Errorf("createCompletionMarker() put key = %v, want %v", gotKey, tt.wantKey)
			}
		})
	}
}

func TestUploadSchemaToS3(t *testing.T) {
	tests := []struct {
		name    string
		bucket  string
		key     string
		schema  []byte
		putErr  error
		wantErr bool
	}{
		{
			name:    "successful upload",
			bucket:  "test-bucket",
			key:     "schemas/v1/exported.sql",
			schema:  []byte("CREATE TABLE users (id INT);"),
			putErr:  nil,
			wantErr: false,
		},
		{
			name:    "upload error",
			bucket:  "test-bucket",
			key:     "schemas/v1/exported.sql",
			schema:  []byte("CREATE TABLE users (id INT);"),
			putErr:  fmt.Errorf("access denied"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotBucket, gotKey string
			mock := &mockS3Client{
				putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
					if tt.putErr != nil {
						return nil, tt.putErr
					}
					gotBucket = *params.Bucket
					gotKey = *params.Key
					return &s3.PutObjectOutput{}, nil
				},
			}

			err := uploadSchemaToS3(context.Background(), mock, tt.bucket, tt.key, tt.schema)
			if (err != nil) != tt.wantErr {
				t.Errorf("uploadSchemaToS3() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotBucket != tt.bucket {
					t.Errorf("uploadSchemaToS3() bucket = %v, want %v", gotBucket, tt.bucket)
				}
				if gotKey != tt.key {
					t.Errorf("uploadSchemaToS3() key = %v, want %v", gotKey, tt.key)
				}
			}
		})
	}
}

func TestFindLatestSchemaWithMock(t *testing.T) {
	tests := []struct {
		name           string
		bucket         string
		prefix         string
		schemaFileName string
		objects        []string
		listErr        error
		wantKey        string
		wantVersion    string
		wantErr        bool
	}{
		{
			name:           "finds latest version",
			bucket:         "test-bucket",
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			objects:        []string{"schemas/v1/schema.sql", "schemas/v2/schema.sql", "schemas/v3/schema.sql"},
			wantKey:        "schemas/v3/schema.sql",
			wantVersion:    "v3",
			wantErr:        false,
		},
		{
			name:           "handles semver correctly",
			bucket:         "test-bucket",
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			objects:        []string{"schemas/v1.0.0/schema.sql", "schemas/v1.10.0/schema.sql", "schemas/v2.0.0/schema.sql"},
			wantKey:        "schemas/v2.0.0/schema.sql",
			wantVersion:    "v2.0.0",
			wantErr:        false,
		},
		{
			name:           "list error",
			bucket:         "test-bucket",
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			listErr:        fmt.Errorf("access denied"),
			wantErr:        true,
		},
		{
			name:           "no schema files",
			bucket:         "test-bucket",
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			objects:        []string{"schemas/v1/other.sql"},
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockS3Client{
				listObjectsFunc: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
					if tt.listErr != nil {
						return nil, tt.listErr
					}
					var contents []types.Object
					for _, key := range tt.objects {
						keyCopy := key
						contents = append(contents, types.Object{Key: aws.String(keyCopy)})
					}
					return &s3.ListObjectsV2Output{Contents: contents}, nil
				},
			}

			gotKey, gotVersion, err := findLatestSchema(context.Background(), mock, tt.bucket, tt.prefix, tt.schemaFileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("findLatestSchema() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotKey != tt.wantKey {
					t.Errorf("findLatestSchema() key = %v, want %v", gotKey, tt.wantKey)
				}
				if gotVersion != tt.wantVersion {
					t.Errorf("findLatestSchema() version = %v, want %v", gotVersion, tt.wantVersion)
				}
			}
		})
	}
}

func TestFindLatestCompletedSchemaWithMock(t *testing.T) {
	tests := []struct {
		name              string
		bucket            string
		prefix            string
		schemaFileName    string
		completedFileName string
		objects           []string
		listErr           error
		wantKey           string
		wantVersion       string
		wantErr           bool
	}{
		{
			name:              "finds latest completed version",
			bucket:            "test-bucket",
			prefix:            "schemas/",
			schemaFileName:    "schema.sql",
			completedFileName: "completed",
			objects: []string{
				"schemas/v1/schema.sql",
				"schemas/v1/completed",
				"schemas/v2/schema.sql",
				"schemas/v2/completed",
				"schemas/v3/schema.sql", // no completion marker
			},
			wantKey:     "schemas/v2/schema.sql",
			wantVersion: "v2",
			wantErr:     false,
		},
		{
			name:              "no completed schemas",
			bucket:            "test-bucket",
			prefix:            "schemas/",
			schemaFileName:    "schema.sql",
			completedFileName: "completed",
			objects: []string{
				"schemas/v1/schema.sql",
				"schemas/v2/schema.sql",
			},
			wantErr: true,
		},
		{
			name:              "list error",
			bucket:            "test-bucket",
			prefix:            "schemas/",
			schemaFileName:    "schema.sql",
			completedFileName: "completed",
			listErr:           fmt.Errorf("access denied"),
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockS3Client{
				listObjectsFunc: func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
					if tt.listErr != nil {
						return nil, tt.listErr
					}
					var contents []types.Object
					for _, key := range tt.objects {
						keyCopy := key
						contents = append(contents, types.Object{Key: aws.String(keyCopy)})
					}
					return &s3.ListObjectsV2Output{Contents: contents}, nil
				},
			}

			gotKey, gotVersion, err := findLatestCompletedSchema(context.Background(), mock, tt.bucket, tt.prefix, tt.schemaFileName, tt.completedFileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("findLatestCompletedSchema() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotKey != tt.wantKey {
					t.Errorf("findLatestCompletedSchema() key = %v, want %v", gotKey, tt.wantKey)
				}
				if gotVersion != tt.wantVersion {
					t.Errorf("findLatestCompletedSchema() version = %v, want %v", gotVersion, tt.wantVersion)
				}
			}
		})
	}
}

func TestRunCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		{
			name:    "successful command",
			command: "true",
			wantErr: false,
		},
		{
			name:    "failing command",
			command: "false",
			wantErr: true,
		},
		{
			name:    "echo command",
			command: "echo hello",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runCommand(tt.command)
			if (err != nil) != tt.wantErr {
				t.Errorf("runCommand() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRunCommandWithEnv(t *testing.T) {
	tests := []struct {
		name    string
		command string
		hookEnv *HookEnv
		wantErr bool
	}{
		{
			name:    "with nil hookEnv",
			command: "true",
			hookEnv: nil,
			wantErr: false,
		},
		{
			name:    "with hookEnv",
			command: "true",
			hookEnv: &HookEnv{
				S3Bucket: "test-bucket",
				Version:  "v1.0.0",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runCommandWithEnv(tt.command, tt.hookEnv)
			if (err != nil) != tt.wantErr {
				t.Errorf("runCommandWithEnv() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCompareVersions_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		{
			name: "invalid vs valid version - string comparison (i < v)",
			v1:   "invalid",
			v2:   "v1",
			want: -1,
		},
		{
			name: "valid vs invalid version - string comparison (v > i)",
			v1:   "v1",
			v2:   "invalid",
			want: 1,
		},
		{
			name: "both invalid - string comparison (abc < def)",
			v1:   "abc",
			v2:   "def",
			want: -1,
		},
		{
			name: "both invalid - equal",
			v1:   "same",
			v2:   "same",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %v, want %v", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestFindMaxVersion_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		want     string
		wantErr  bool
	}{
		{
			name:     "all invalid versions",
			versions: []string{"invalid1", "invalid2", "invalid3"},
			wantErr:  true,
		},
		{
			name:     "mixed valid and invalid",
			versions: []string{"invalid", "v1", "v2"},
			want:     "v2",
			wantErr:  false,
		},
		{
			name:     "single valid version",
			versions: []string{"v1"},
			want:     "v1",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findMaxVersion(tt.versions)
			if (err != nil) != tt.wantErr {
				t.Errorf("findMaxVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("findMaxVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
