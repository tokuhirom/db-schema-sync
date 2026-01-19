package main

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	applyTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "db_schema_sync_apply_total",
		Help: "Total number of schema apply attempts",
	})

	applySuccessTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "db_schema_sync_apply_success_total",
		Help: "Total number of successful schema applies",
	})

	applyErrorTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "db_schema_sync_apply_error_total",
		Help: "Total number of failed schema applies",
	})

	s3FetchTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "db_schema_sync_s3_fetch_total",
		Help: "Total number of S3 fetch attempts",
	})

	s3FetchErrorTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "db_schema_sync_s3_fetch_error_total",
		Help: "Total number of S3 fetch errors",
	})

	consecutiveFailures = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "db_schema_sync_consecutive_failures",
		Help: "Current number of consecutive failures",
	})

	lastApplyTimestamp = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "db_schema_sync_last_apply_timestamp_seconds",
		Help: "Unix timestamp of the last successful schema apply",
	})

	processStartTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "db_schema_sync_process_start_time_seconds",
		Help: "Unix timestamp when the process started",
	})

	lastAppliedVersionInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "db_schema_sync_last_applied_version_info",
		Help: "Information about the last applied schema version",
	}, []string{"version"})
)

func init() {
	prometheus.MustRegister(applyTotal)
	prometheus.MustRegister(applySuccessTotal)
	prometheus.MustRegister(applyErrorTotal)
	prometheus.MustRegister(s3FetchTotal)
	prometheus.MustRegister(s3FetchErrorTotal)
	prometheus.MustRegister(consecutiveFailures)
	prometheus.MustRegister(lastApplyTimestamp)
	prometheus.MustRegister(processStartTime)
	prometheus.MustRegister(lastAppliedVersionInfo)
}

// startMetricsServer starts an HTTP server for Prometheus metrics
func startMetricsServer(addr string) {
	if addr == "" {
		return
	}

	// Record process start time
	processStartTime.Set(float64(time.Now().Unix()))

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

	slog.Info("Starting metrics server", "addr", addr, "metrics", "http://"+addr+"/metrics", "health", "http://"+addr+"/health")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Metrics server error", "error", err)
	}
}

// recordS3FetchAttempt records an S3 fetch attempt
func recordS3FetchAttempt() {
	s3FetchTotal.Inc()
}

// recordS3FetchError records an S3 fetch error
func recordS3FetchError() {
	s3FetchErrorTotal.Inc()
}

// recordApplyAttempt records a schema apply attempt
func recordApplyAttempt() {
	applyTotal.Inc()
}

// recordApplySuccess records a successful schema apply
func recordApplySuccess(version string) {
	applySuccessTotal.Inc()
	lastApplyTimestamp.Set(float64(time.Now().Unix()))
	// Reset the previous version labels and set the new one
	lastAppliedVersionInfo.Reset()
	lastAppliedVersionInfo.WithLabelValues(version).Set(1)
}

// recordApplyError records a schema apply error
func recordApplyError() {
	applyErrorTotal.Inc()
}

// recordConsecutiveFailures updates the consecutive failures gauge
func recordConsecutiveFailures(count int) {
	consecutiveFailures.Set(float64(count))
}
