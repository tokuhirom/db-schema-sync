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

func TestFindLatestCompletedSchema(t *testing.T) {
	tests := []struct {
		name              string
		objects           []string
		prefix            string
		schemaFileName    string
		completedFileName string
		wantKey           string
		wantVersion       string
		wantErr           bool
	}{
		{
			name: "finds latest completed schema",
			objects: []string{
				"schemas/v1/schema.sql",
				"schemas/v1/completed",
				"schemas/v2/schema.sql",
				"schemas/v2/completed",
				"schemas/v3/schema.sql",
				// v3 has no completed marker
			},
			prefix:            "schemas/",
			schemaFileName:    "schema.sql",
			completedFileName: "completed",
			wantKey:           "schemas/v2/schema.sql",
			wantVersion:       "v2",
			wantErr:           false,
		},
		{
			name: "only one completed version",
			objects: []string{
				"schemas/v1/schema.sql",
				"schemas/v1/completed",
				"schemas/v2/schema.sql",
				// v2 has no completed marker
			},
			prefix:            "schemas/",
			schemaFileName:    "schema.sql",
			completedFileName: "completed",
			wantKey:           "schemas/v1/schema.sql",
			wantVersion:       "v1",
			wantErr:           false,
		},
		{
			name: "no completed schemas",
			objects: []string{
				"schemas/v1/schema.sql",
				"schemas/v2/schema.sql",
			},
			prefix:            "schemas/",
			schemaFileName:    "schema.sql",
			completedFileName: "completed",
			wantErr:           true,
		},
		{
			name: "semver sorting for completed schemas",
			objects: []string{
				"schemas/v1.0.0/schema.sql",
				"schemas/v1.0.0/completed",
				"schemas/v1.10.0/schema.sql",
				"schemas/v1.10.0/completed",
				"schemas/v1.2.0/schema.sql",
				"schemas/v1.2.0/completed",
			},
			prefix:            "schemas/",
			schemaFileName:    "schema.sql",
			completedFileName: "completed",
			wantKey:           "schemas/v1.10.0/schema.sql",
			wantVersion:       "v1.10.0",
			wantErr:           false,
		},
		{
			name:              "empty bucket",
			objects:           []string{},
			prefix:            "schemas/",
			schemaFileName:    "schema.sql",
			completedFileName: "completed",
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock S3 client
			mockClient := &mockS3Client{
				listObjectsFunc: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
					var contents []types.Object
					for _, key := range tt.objects {
						keyCopy := key
						contents = append(contents, types.Object{Key: &keyCopy})
					}
					return &s3.ListObjectsV2Output{Contents: contents}, nil
				},
			}

			gotKey, gotVersion, err := findLatestCompletedSchema(context.Background(), mockClient, "test-bucket", tt.prefix, tt.schemaFileName, tt.completedFileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("findLatestCompletedSchema() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotKey != tt.wantKey {
					t.Errorf("findLatestCompletedSchema() gotKey = %v, want %v", gotKey, tt.wantKey)
				}
				if gotVersion != tt.wantVersion {
					t.Errorf("findLatestCompletedSchema() gotVersion = %v, want %v", gotVersion, tt.wantVersion)
				}
			}
		})
	}
}

func TestFindLatestCompletedSchema_S3Error(t *testing.T) {
	mockClient := &mockS3Client{
		listObjectsFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, fmt.Errorf("S3 connection error")
		},
	}

	_, _, err := findLatestCompletedSchema(context.Background(), mockClient, "test-bucket", "schemas/", "schema.sql", "completed")
	if err == nil {
		t.Error("expected error from findLatestCompletedSchema when S3 fails")
	}
}

func TestBuildExportedSchemaKey(t *testing.T) {
	tests := []struct {
		name      string
		schemaKey string
		want      string
	}{
		{
			name:      "simple path",
			schemaKey: "schemas/v1/schema.sql",
			want:      "schemas/v1/exported.sql",
		},
		{
			name:      "nested path",
			schemaKey: "prod/schemas/v1.0.0/schema.sql",
			want:      "prod/schemas/v1.0.0/exported.sql",
		},
		{
			name:      "different filename",
			schemaKey: "data/v2/myschema.sql",
			want:      "data/v2/exported.sql",
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
		schemaData  string
		expectError bool
	}{
		{
			name:        "successful download",
			schemaData:  "CREATE TABLE users (id INT PRIMARY KEY);",
			expectError: false,
		},
		{
			name:        "empty schema",
			schemaData:  "",
			expectError: false,
		},
		{
			name:        "large schema",
			schemaData:  strings.Repeat("CREATE TABLE test (id INT);\n", 1000),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockS3Client{
				getObjectFunc: func(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
					return &s3.GetObjectOutput{
						Body: io.NopCloser(strings.NewReader(tt.schemaData)),
					}, nil
				},
			}

			result, err := downloadSchemaFromS3(context.Background(), mockClient, "test-bucket", "schemas/v1/schema.sql")
			if (err != nil) != tt.expectError {
				t.Errorf("downloadSchemaFromS3() error = %v, expectError %v", err, tt.expectError)
				return
			}
			if !tt.expectError && string(result) != tt.schemaData {
				t.Errorf("downloadSchemaFromS3() = %v, want %v", string(result), tt.schemaData)
			}
		})
	}
}

func TestDownloadSchemaFromS3_Error(t *testing.T) {
	mockClient := &mockS3Client{
		getObjectFunc: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, fmt.Errorf("access denied")
		},
	}

	_, err := downloadSchemaFromS3(context.Background(), mockClient, "test-bucket", "schemas/v1/schema.sql")
	if err == nil {
		t.Error("expected error from downloadSchemaFromS3 when S3 fails")
	}
}

func TestCheckCompletionMarker(t *testing.T) {
	tests := []struct {
		name              string
		headObjectErr     error
		wantExists        bool
		wantErr           bool
	}{
		{
			name:          "marker exists",
			headObjectErr: nil,
			wantExists:    true,
			wantErr:       false,
		},
		{
			name:          "marker not found",
			headObjectErr: fmt.Errorf("NotFound: The specified key does not exist"),
			wantExists:    false,
			wantErr:       false,
		},
		{
			name:          "S3 error",
			headObjectErr: fmt.Errorf("access denied"),
			wantExists:    false,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockS3Client{
				headObjectFunc: func(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
					if tt.headObjectErr != nil {
						return nil, tt.headObjectErr
					}
					return &s3.HeadObjectOutput{}, nil
				},
			}

			exists, err := checkCompletionMarker(context.Background(), mockClient, "test-bucket", "schemas/v1/schema.sql", "completed")
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
		name        string
		putError    error
		wantErr     bool
	}{
		{
			name:     "successful creation",
			putError: nil,
			wantErr:  false,
		},
		{
			name:     "S3 error",
			putError: fmt.Errorf("access denied"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedKey string
			mockClient := &mockS3Client{
				putObjectFunc: func(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
					capturedKey = *params.Key
					if tt.putError != nil {
						return nil, tt.putError
					}
					return &s3.PutObjectOutput{}, nil
				},
			}

			err := createCompletionMarker(context.Background(), mockClient, "test-bucket", "schemas/v1/schema.sql", "completed")
			if (err != nil) != tt.wantErr {
				t.Errorf("createCompletionMarker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && capturedKey != "schemas/v1/completed" {
				t.Errorf("createCompletionMarker() uploaded to key = %v, want %v", capturedKey, "schemas/v1/completed")
			}
		})
	}
}

func TestUploadSchemaToS3(t *testing.T) {
	tests := []struct {
		name     string
		schema   []byte
		putError error
		wantErr  bool
	}{
		{
			name:     "successful upload",
			schema:   []byte("CREATE TABLE users (id INT);"),
			putError: nil,
			wantErr:  false,
		},
		{
			name:     "empty schema",
			schema:   []byte{},
			putError: nil,
			wantErr:  false,
		},
		{
			name:     "S3 error",
			schema:   []byte("CREATE TABLE users (id INT);"),
			putError: fmt.Errorf("access denied"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedKey string
			var capturedBody string
			mockClient := &mockS3Client{
				putObjectFunc: func(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
					capturedKey = *params.Key
					if params.Body != nil {
						body, _ := io.ReadAll(params.Body)
						capturedBody = string(body)
					}
					if tt.putError != nil {
						return nil, tt.putError
					}
					return &s3.PutObjectOutput{}, nil
				},
			}

			err := uploadSchemaToS3(context.Background(), mockClient, "test-bucket", "schemas/v1/exported.sql", tt.schema)
			if (err != nil) != tt.wantErr {
				t.Errorf("uploadSchemaToS3() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if capturedKey != "schemas/v1/exported.sql" {
					t.Errorf("uploadSchemaToS3() uploaded to key = %v, want %v", capturedKey, "schemas/v1/exported.sql")
				}
				if capturedBody != string(tt.schema) {
					t.Errorf("uploadSchemaToS3() uploaded body = %v, want %v", capturedBody, string(tt.schema))
				}
			}
		})
	}
}

func TestFindLatestSchema(t *testing.T) {
	tests := []struct {
		name           string
		objects        []string
		prefix         string
		schemaFileName string
		wantKey        string
		wantVersion    string
		wantErr        bool
	}{
		{
			name: "finds latest schema",
			objects: []string{
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
			name:           "empty bucket",
			objects:        []string{},
			prefix:         "schemas/",
			schemaFileName: "schema.sql",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockS3Client{
				listObjectsFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
					var contents []types.Object
					for _, key := range tt.objects {
						keyCopy := key
						contents = append(contents, types.Object{Key: &keyCopy})
					}
					return &s3.ListObjectsV2Output{Contents: contents}, nil
				},
			}

			gotKey, gotVersion, err := findLatestSchema(context.Background(), mockClient, "test-bucket", tt.prefix, tt.schemaFileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("findLatestSchema() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotKey != tt.wantKey {
					t.Errorf("findLatestSchema() gotKey = %v, want %v", gotKey, tt.wantKey)
				}
				if gotVersion != tt.wantVersion {
					t.Errorf("findLatestSchema() gotVersion = %v, want %v", gotVersion, tt.wantVersion)
				}
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
			name:    "simple command without env",
			command: "true",
			hookEnv: nil,
			wantErr: false,
		},
		{
			name:    "command with hook env",
			command: "test -n \"$DB_SCHEMA_SYNC_S3_BUCKET\"",
			hookEnv: &HookEnv{
				S3Bucket: "test-bucket",
			},
			wantErr: false,
		},
		{
			name:    "failing command",
			command: "false",
			hookEnv: nil,
			wantErr: true,
		},
		{
			name:    "command using multiple env vars",
			command: "test \"$DB_SCHEMA_SYNC_S3_BUCKET\" = \"my-bucket\" && test \"$DB_SCHEMA_SYNC_VERSION\" = \"v1.0.0\"",
			hookEnv: &HookEnv{
				S3Bucket: "my-bucket",
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

func TestRunCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr bool
	}{
		{
			name:    "successful command",
			command: "echo hello",
			wantErr: false,
		},
		{
			name:    "failing command",
			command: "exit 1",
			wantErr: true,
		},
		{
			name:    "command not found",
			command: "nonexistent_command_12345",
			wantErr: true,
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

// Helper function to create S3 objects list for testing
func createS3Objects(keys []string) []types.Object {
	var objects []types.Object
	for _, key := range keys {
		keyCopy := key
		objects = append(objects, types.Object{Key: aws.String(keyCopy)})
	}
	return objects
}

func TestRunHook(t *testing.T) {
	tests := []struct {
		name    string
		command string
		hookEnv *HookEnv
	}{
		{
			name:    "empty command should not run",
			command: "",
			hookEnv: nil,
		},
		{
			name:    "successful hook",
			command: "true",
			hookEnv: &HookEnv{S3Bucket: "test-bucket"},
		},
		{
			name:    "failing hook should log error but not panic",
			command: "false",
			hookEnv: &HookEnv{S3Bucket: "test-bucket"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// runHook should not panic
			runHook(tt.name, tt.command, tt.hookEnv)
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
			name:     "single version",
			versions: []string{"v1"},
			want:     "v1",
			wantErr:  false,
		},
		{
			name:     "all invalid versions",
			versions: []string{"invalid", "not-a-version"},
			wantErr:  true,
		},
		{
			name:     "mix of valid and invalid",
			versions: []string{"invalid", "v1", "not-a-version", "v2"},
			want:     "v2",
			wantErr:  false,
		},
		{
			name:     "pre-release versions",
			versions: []string{"v1.0.0-alpha", "v1.0.0-beta", "v1.0.0"},
			want:     "v1.0.0",
			wantErr:  false,
		},
		{
			name:     "large version numbers",
			versions: []string{"v100.200.300", "v1.2.3", "v99.99.99"},
			want:     "v100.200.300",
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

func TestCompareVersions_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		{
			name: "invalid v1",
			v1:   "invalid",
			v2:   "v1",
			want: -1, // string comparison: "invalid" < "v1" (i < v in ASCII)
		},
		{
			name: "invalid v2",
			v1:   "v1",
			v2:   "invalid",
			want: 1, // string comparison: "v1" > "invalid"
		},
		{
			name: "both invalid",
			v1:   "abc",
			v2:   "def",
			want: -1, // string comparison: "abc" < "def"
		},
		{
			name: "equal invalid",
			v1:   "invalid",
			v2:   "invalid",
			want: 0,
		},
		{
			name: "pre-release vs release",
			v1:   "v1.0.0-alpha",
			v2:   "v1.0.0",
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

func TestFindLatestSchema_S3Error(t *testing.T) {
	mockClient := &mockS3Client{
		listObjectsFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, fmt.Errorf("S3 connection error")
		},
	}

	_, _, err := findLatestSchema(context.Background(), mockClient, "test-bucket", "schemas/", "schema.sql")
	if err == nil {
		t.Error("expected error from findLatestSchema when S3 fails")
	}
}

func TestFindLatestCompletedSchema_WithDifferentMarkerNames(t *testing.T) {
	objects := []string{
		"schemas/v1/schema.sql",
		"schemas/v1/.done",
		"schemas/v2/schema.sql",
		"schemas/v2/.done",
	}

	mockClient := &mockS3Client{
		listObjectsFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			var contents []types.Object
			for _, key := range objects {
				keyCopy := key
				contents = append(contents, types.Object{Key: &keyCopy})
			}
			return &s3.ListObjectsV2Output{Contents: contents}, nil
		},
	}

	gotKey, gotVersion, err := findLatestCompletedSchema(context.Background(), mockClient, "test-bucket", "schemas/", "schema.sql", ".done")
	if err != nil {
		t.Errorf("findLatestCompletedSchema() error = %v", err)
		return
	}
	if gotKey != "schemas/v2/schema.sql" {
		t.Errorf("findLatestCompletedSchema() gotKey = %v, want %v", gotKey, "schemas/v2/schema.sql")
	}
	if gotVersion != "v2" {
		t.Errorf("findLatestCompletedSchema() gotVersion = %v, want %v", gotVersion, "v2")
	}
}
