package main

import (
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
