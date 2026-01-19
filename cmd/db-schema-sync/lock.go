package main

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// AdvisoryLockID はスキーマ適用時に使用するロックID
// "DBSCHEMA" を16進数で表現した値
const AdvisoryLockID int64 = 0x4442534348454D41

// AdvisoryLocker はPostgreSQL Advisory Lockを管理する
type AdvisoryLocker struct {
	db *sql.DB
}

// NewAdvisoryLocker は新しいAdvisoryLockerを作成する
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

// TryLock は非ブロッキングでロック取得を試みる
// 取得成功: true, nil / 既にロック中: false, nil / エラー: false, error
func (l *AdvisoryLocker) TryLock(ctx context.Context) (bool, error) {
	var acquired bool
	err := l.db.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", AdvisoryLockID).Scan(&acquired)
	if err != nil {
		return false, fmt.Errorf("failed to acquire advisory lock: %w", err)
	}
	return acquired, nil
}

// Unlock はロックを解放する
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

// Close は接続を閉じる（ロックも自動解放される）
func (l *AdvisoryLocker) Close() error {
	return l.db.Close()
}
