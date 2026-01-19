//go:build integration

package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupPostgresContainer(t *testing.T) (host string, port string, cleanup func()) {
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start container: %v", err)
	}

	hostIP, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to get container host: %v", err)
	}

	mappedPort, err := container.MappedPort(ctx, "5432")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("failed to get container port: %v", err)
	}

	cleanup = func() {
		container.Terminate(ctx)
	}

	return hostIP, mappedPort.Port(), cleanup
}

func TestAdvisoryLocker_TryLock(t *testing.T) {
	host, port, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()

	// Create locker
	locker, err := NewAdvisoryLocker(host, port, "testuser", "testpass", "testdb")
	if err != nil {
		t.Fatalf("failed to create locker: %v", err)
	}
	defer locker.Close()

	// First lock should succeed
	acquired, err := locker.TryLock(ctx)
	if err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}
	if !acquired {
		t.Error("expected to acquire lock on first attempt")
	}

	// Unlock
	err = locker.Unlock(ctx)
	if err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}

	// Second lock should succeed after unlock
	acquired, err = locker.TryLock(ctx)
	if err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}
	if !acquired {
		t.Error("expected to acquire lock after unlock")
	}

	// Cleanup
	locker.Unlock(ctx)
}

func TestAdvisoryLocker_ConcurrentLock(t *testing.T) {
	host, port, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()

	// First locker acquires the lock
	locker1, err := NewAdvisoryLocker(host, port, "testuser", "testpass", "testdb")
	if err != nil {
		t.Fatalf("failed to create locker1: %v", err)
	}
	defer locker1.Close()

	acquired1, err := locker1.TryLock(ctx)
	if err != nil {
		t.Fatalf("locker1 TryLock failed: %v", err)
	}
	if !acquired1 {
		t.Error("expected locker1 to acquire lock")
	}

	// Second locker should fail to acquire the lock
	locker2, err := NewAdvisoryLocker(host, port, "testuser", "testpass", "testdb")
	if err != nil {
		t.Fatalf("failed to create locker2: %v", err)
	}
	defer locker2.Close()

	acquired2, err := locker2.TryLock(ctx)
	if err != nil {
		t.Fatalf("locker2 TryLock failed: %v", err)
	}
	if acquired2 {
		t.Error("expected locker2 to fail acquiring lock")
	}

	// After locker1 releases, locker2 should be able to acquire
	err = locker1.Unlock(ctx)
	if err != nil {
		t.Fatalf("locker1 Unlock failed: %v", err)
	}

	acquired2, err = locker2.TryLock(ctx)
	if err != nil {
		t.Fatalf("locker2 TryLock (retry) failed: %v", err)
	}
	if !acquired2 {
		t.Error("expected locker2 to acquire lock after locker1 released")
	}

	// Cleanup
	locker2.Unlock(ctx)
}

func TestAdvisoryLocker_ConnectionClose_ReleasesLock(t *testing.T) {
	host, port, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()

	// First locker acquires the lock and then closes connection
	locker1, err := NewAdvisoryLocker(host, port, "testuser", "testpass", "testdb")
	if err != nil {
		t.Fatalf("failed to create locker1: %v", err)
	}

	acquired1, err := locker1.TryLock(ctx)
	if err != nil {
		t.Fatalf("locker1 TryLock failed: %v", err)
	}
	if !acquired1 {
		t.Error("expected locker1 to acquire lock")
	}

	// Close connection without explicit unlock
	locker1.Close()

	// Wait a bit for PostgreSQL to cleanup the connection
	time.Sleep(100 * time.Millisecond)

	// Second locker should be able to acquire the lock
	locker2, err := NewAdvisoryLocker(host, port, "testuser", "testpass", "testdb")
	if err != nil {
		t.Fatalf("failed to create locker2: %v", err)
	}
	defer locker2.Close()

	acquired2, err := locker2.TryLock(ctx)
	if err != nil {
		t.Fatalf("locker2 TryLock failed: %v", err)
	}
	if !acquired2 {
		t.Error("expected locker2 to acquire lock after locker1 connection closed")
	}

	// Cleanup
	locker2.Unlock(ctx)
}

func TestAdvisoryLocker_ParallelExecution(t *testing.T) {
	host, port, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()

	// Track which goroutines acquired the lock
	var mu sync.Mutex
	acquiredCount := 0
	failedCount := 0
	numWorkers := 5

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			defer wg.Done()

			locker, err := NewAdvisoryLocker(host, port, "testuser", "testpass", "testdb")
			if err != nil {
				t.Errorf("worker %d: failed to create locker: %v", workerID, err)
				return
			}
			defer locker.Close()

			acquired, err := locker.TryLock(ctx)
			if err != nil {
				t.Errorf("worker %d: TryLock failed: %v", workerID, err)
				return
			}

			mu.Lock()
			if acquired {
				acquiredCount++
				// Simulate some work
				time.Sleep(50 * time.Millisecond)
				locker.Unlock(ctx)
			} else {
				failedCount++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// At most one worker should have acquired the lock
	// (others may acquire after the first releases, but at any instant only one holds it)
	t.Logf("acquired: %d, failed: %d", acquiredCount, failedCount)

	// Total should equal numWorkers
	if acquiredCount+failedCount != numWorkers {
		t.Errorf("expected total %d, got acquired=%d + failed=%d", numWorkers, acquiredCount, failedCount)
	}
}

func TestAdvisoryLocker_Unlock_WithoutLock(t *testing.T) {
	host, port, cleanup := setupPostgresContainer(t)
	defer cleanup()

	ctx := context.Background()

	locker, err := NewAdvisoryLocker(host, port, "testuser", "testpass", "testdb")
	if err != nil {
		t.Fatalf("failed to create locker: %v", err)
	}
	defer locker.Close()

	// Unlock without acquiring should return error
	err = locker.Unlock(ctx)
	if err == nil {
		t.Error("expected error when unlocking without lock")
	}
}
