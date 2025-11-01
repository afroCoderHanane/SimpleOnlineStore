package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// NewMySQLDB initializes a connection pool to a MySQL database using environment variables.
// Required environment variables:
//   - DB_USER
//   - DB_PASSWORD (can be empty for local dev)
//   - DB_NAME
//
// Optional environment variables with defaults:
//   - DB_HOST (default: localhost)
//   - DB_PORT (default: 3306)
//   - DB_MAX_OPEN_CONNS
//   - DB_MAX_IDLE_CONNS
//   - DB_CONN_MAX_LIFETIME (duration, e.g. "30m")
func NewMySQLDB(ctx context.Context) (*sql.DB, error) {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "3306")
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASSWORD")
	name := os.Getenv("DB_NAME")

	if user == "" {
		return nil, fmt.Errorf("DB_USER must be set")
	}
	if name == "" {
		return nil, fmt.Errorf("DB_NAME must be set")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true", user, pass, host, port, name)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	if err := configurePool(db); err != nil {
		db.Close()
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return db, nil
}

func configurePool(db *sql.DB) error {
	if v := os.Getenv("DB_MAX_OPEN_CONNS"); v != "" {
		maxOpen, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid DB_MAX_OPEN_CONNS: %w", err)
		}
		db.SetMaxOpenConns(maxOpen)
	}

	if v := os.Getenv("DB_MAX_IDLE_CONNS"); v != "" {
		maxIdle, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid DB_MAX_IDLE_CONNS: %w", err)
		}
		db.SetMaxIdleConns(maxIdle)
	}

	if v := os.Getenv("DB_CONN_MAX_LIFETIME"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid DB_CONN_MAX_LIFETIME: %w", err)
		}
		db.SetConnMaxLifetime(d)
	}

	return nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
