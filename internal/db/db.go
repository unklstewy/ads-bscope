package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/unklstewy/ads-bscope/pkg/config"
)

//go:embed schema.sql
var schemaSQL embed.FS

// DB wraps a database connection with helper methods.
type DB struct {
	*sql.DB
	config config.DatabaseConfig
}

// Connect establishes a connection to the PostgreSQL database.
func Connect(cfg config.DatabaseConfig) (*DB, error) {
	// Build connection string
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host,
		cfg.Port,
		cfg.Username,
		cfg.Password,
		cfg.Database,
		cfg.SSLMode,
	)

	// Open connection
	sqlDB, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{
		DB:     sqlDB,
		config: cfg,
	}

	return db, nil
}

// InitSchema creates or updates the database schema.
// This should be called once at application startup.
func (db *DB) InitSchema(ctx context.Context) error {
	// Read schema SQL
	schemaBytes, err := schemaSQL.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	// Execute schema
	if _, err := db.ExecContext(ctx, string(schemaBytes)); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	return nil
}

// CleanupOldData removes stale aircraft and old position history.
// Should be called periodically to prevent unbounded growth.
func (db *DB) CleanupOldData(ctx context.Context, maxAge time.Duration) error {
	cutoff := time.Now().UTC().Add(-maxAge)

	// Mark aircraft as not visible if not seen recently
	_, err := db.ExecContext(ctx,
		`UPDATE aircraft SET is_visible = FALSE WHERE last_seen < $1`,
		cutoff,
	)
	if err != nil {
		return fmt.Errorf("failed to mark stale aircraft: %w", err)
	}

	// Delete old position history (keep last 24 hours)
	positionCutoff := time.Now().UTC().Add(-24 * time.Hour)
	_, err = db.ExecContext(ctx,
		`DELETE FROM aircraft_positions WHERE timestamp < $1`,
		positionCutoff,
	)
	if err != nil {
		return fmt.Errorf("failed to delete old positions: %w", err)
	}

	// Delete aircraft not seen in over 1 hour
	deleteCutoff := time.Now().UTC().Add(-1 * time.Hour)
	_, err = db.ExecContext(ctx,
		`DELETE FROM aircraft WHERE last_seen < $1 AND is_visible = FALSE`,
		deleteCutoff,
	)
	if err != nil {
		return fmt.Errorf("failed to delete old aircraft: %w", err)
	}

	return nil
}

// GetStats returns database statistics.
func (db *DB) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count visible aircraft
	var visibleCount int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM aircraft WHERE is_visible = TRUE`,
	).Scan(&visibleCount)
	if err != nil {
		return nil, err
	}
	stats["visible_aircraft"] = visibleCount

	// Count trackable aircraft
	var trackableCount int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM aircraft WHERE is_trackable = TRUE AND is_visible = TRUE`,
	).Scan(&trackableCount)
	if err != nil {
		return nil, err
	}
	stats["trackable_aircraft"] = trackableCount

	// Count approaching aircraft
	var approachingCount int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM aircraft WHERE is_approaching = TRUE AND is_visible = TRUE`,
	).Scan(&approachingCount)
	if err != nil {
		return nil, err
	}
	stats["approaching_aircraft"] = approachingCount

	// Total position records
	var positionCount int64
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM aircraft_positions`,
	).Scan(&positionCount)
	if err != nil {
		return nil, err
	}
	stats["position_records"] = positionCount

	return stats, nil
}
