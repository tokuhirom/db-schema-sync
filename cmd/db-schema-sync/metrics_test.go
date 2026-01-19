package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// getFreePort returns a free port for testing
func getFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

// startTestMetricsServer starts a metrics server for testing and returns the address
func startTestMetricsServer(t *testing.T) (string, func()) {
	t.Helper()
	port := getFreePort(t)
	addr := fmt.Sprintf(":%d", port)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		_ = server.ListenAndServe()
	}()

	// Wait for server to start
	baseURL := fmt.Sprintf("http://localhost%s", addr)
	for i := 0; i < 50; i++ {
		resp, err := http.Get(baseURL + "/health")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}

	return baseURL, cleanup
}

func TestMetricsEndpoint(t *testing.T) {
	baseURL, cleanup := startTestMetricsServer(t)
	defer cleanup()

	resp, err := http.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("failed to get /metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	// Check that our custom metrics are present
	expectedMetrics := []string{
		"db_schema_sync_apply_total",
		"db_schema_sync_apply_success_total",
		"db_schema_sync_apply_error_total",
		"db_schema_sync_s3_fetch_total",
		"db_schema_sync_s3_fetch_error_total",
		"db_schema_sync_consecutive_failures",
		"db_schema_sync_last_apply_timestamp_seconds",
		"db_schema_sync_process_start_time_seconds",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(string(body), metric) {
			t.Errorf("expected metric %q not found in /metrics response", metric)
		}
	}

	// Check that Go runtime metrics are present (provided by prometheus/client_golang)
	if !strings.Contains(string(body), "go_goroutines") {
		t.Error("expected go_goroutines metric not found")
	}
}

func TestHealthEndpoint(t *testing.T) {
	baseURL, cleanup := startTestMetricsServer(t)
	defer cleanup()

	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("failed to get /health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if string(body) != "OK" {
		t.Errorf("expected body 'OK', got %q", string(body))
	}
}

func TestMetricsRecording(t *testing.T) {
	// Create a new registry for isolated testing
	reg := prometheus.NewRegistry()

	testApplyTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_apply_total",
		Help: "Test counter",
	})
	testS3FetchTotal := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_s3_fetch_total",
		Help: "Test counter",
	})
	testConsecutiveFailures := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_consecutive_failures",
		Help: "Test gauge",
	})

	reg.MustRegister(testApplyTotal)
	reg.MustRegister(testS3FetchTotal)
	reg.MustRegister(testConsecutiveFailures)

	// Simulate recording metrics
	testS3FetchTotal.Inc()
	testApplyTotal.Inc()
	testConsecutiveFailures.Set(0)

	// Verify the metrics can be gathered
	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	if len(metrics) != 3 {
		t.Errorf("expected 3 metrics, got %d", len(metrics))
	}
}

// mockS3Client implements S3Client interface for testing
type mockS3ClientForMetrics struct {
	listObjectsFunc func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	getObjectFunc   func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	headObjectFunc  func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	putObjectFunc   func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

func (m *mockS3ClientForMetrics) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.listObjectsFunc != nil {
		return m.listObjectsFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockS3ClientForMetrics) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getObjectFunc != nil {
		return m.getObjectFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockS3ClientForMetrics) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if m.headObjectFunc != nil {
		return m.headObjectFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockS3ClientForMetrics) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putObjectFunc != nil {
		return m.putObjectFunc(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("not implemented")
}

func TestMetricsWithRunSync(t *testing.T) {
	// Start metrics server
	baseURL, cleanup := startTestMetricsServer(t)
	defer cleanup()

	// Create a mock S3 client that returns an error (simulating S3 fetch failure)
	mockClient := &mockS3ClientForMetrics{
		listObjectsFunc: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return nil, fmt.Errorf("simulated S3 error")
		},
	}

	// Reset global state for test
	lastAppliedVersion = ""
	consecutiveFailureCount = 0

	cli := &CLI{
		S3Bucket:   "test-bucket",
		PathPrefix: "schemas/",
		SchemaFile: "schema.sql",
	}

	// Run sync (should fail due to S3 error)
	err := runSync(context.Background(), mockClient, cli, "localhost", "5432", "user", "pass", "db", "", "", "", "")
	if err == nil {
		t.Error("expected error from runSync, got nil")
	}

	// Wait a bit for metrics to be recorded
	time.Sleep(50 * time.Millisecond)

	// Check metrics endpoint
	resp, err := http.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("failed to get /metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	// Verify S3 fetch metrics were recorded
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "db_schema_sync_s3_fetch_total") {
		t.Error("expected db_schema_sync_s3_fetch_total metric not found")
	}
	if !strings.Contains(bodyStr, "db_schema_sync_s3_fetch_error_total") {
		t.Error("expected db_schema_sync_s3_fetch_error_total metric not found")
	}
	if !strings.Contains(bodyStr, "db_schema_sync_consecutive_failures") {
		t.Error("expected db_schema_sync_consecutive_failures metric not found")
	}
}

func TestRecordApplySuccess(t *testing.T) {
	baseURL, cleanup := startTestMetricsServer(t)
	defer cleanup()

	// Record a successful apply
	recordApplySuccess("v1.0.0")

	// Wait for metrics to be recorded
	time.Sleep(50 * time.Millisecond)

	// Check metrics endpoint
	resp, err := http.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("failed to get /metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	bodyStr := string(body)

	// Check that last_applied_version_info has the version label
	if !strings.Contains(bodyStr, `db_schema_sync_last_applied_version_info{version="v1.0.0"}`) {
		t.Error("expected db_schema_sync_last_applied_version_info with version label not found")
	}

	// Check that last_apply_timestamp_seconds is present
	if !strings.Contains(bodyStr, "db_schema_sync_last_apply_timestamp_seconds") {
		t.Error("expected db_schema_sync_last_apply_timestamp_seconds metric not found")
	}
}
