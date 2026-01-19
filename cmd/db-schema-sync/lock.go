package main

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// AdvisoryLockID is the lock ID used during schema application.
// It represents "DBSCHEMA" in hexadecimal.
const AdvisoryLockID int64 = 0x4442534348454D41

// AdvisoryLocker manages PostgreSQL Advisory Locks.
type AdvisoryLocker struct {
	db *sql.DB
}

// NewAdvisoryLocker creates a new AdvisoryLocker.
func NewAdvisoryLocker(dbHost, dbPort, dbUser, dbPassword, dbName string) (*AdvisoryLocker, error) {
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &AdvisoryLocker{db: db}, nil
}

// TryLock attempts to acquire the lock in a non-blocking manner.
// Returns: acquired (true, nil) / already locked (false, nil) / error (false, error)
func (l *AdvisoryLocker) TryLock(ctx context.Context) (bool, error) {
	var acquired bool
	err := l.db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", AdvisoryLockID).Scan(&acquired)
	if err != nil {
		return false, fmt.Errorf("failed to acquire advisory lock: %w", err)
	}
	return acquired, nil
}

// Unlock releases the lock.
func (l *AdvisoryLocker) Unlock(ctx context.Context) error {
	var released bool
	err := l.db.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", AdvisoryLockID).Scan(&released)
	if err != nil {
		return fmt.Errorf("failed to release advisory lock: %w", err)
	}
	if !released {
		return fmt.Errorf("lock was not held")
	}
	return nil
}

// Close closes the connection (lock is automatically released).
func (l *AdvisoryLocker) Close() error {
	return l.db.Close()
}
