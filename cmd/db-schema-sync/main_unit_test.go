package main

import (
	"strings"
	"testing"
)

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
