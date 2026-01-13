package db

import (
	"context"
	"log"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/config"
)

// ReconnectWithRetry attempts to reconnect to the database with exponential backoff.
// This provides resilience against temporary database outages.
//
// Parameters:
//   - cfg: Database configuration
//   - maxRetries: Maximum number of reconnection attempts (0 = infinite)
//   - initialDelay: Initial wait time between retries
//
// Returns: Connected database or error if all retries exhausted
func ReconnectWithRetry(cfg config.DatabaseConfig, maxRetries int, initialDelay time.Duration) (*DB, error) {
	delay := initialDelay
	attempt := 0

	for {
		attempt++

		log.Printf("Database connection attempt %d...", attempt)

		db, err := Connect(cfg)
		if err == nil {
			// Test the connection
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			pingErr := db.PingContext(ctx)
			if pingErr == nil {
				log.Println("✓ Database reconnected successfully")
				return db, nil
			}

			// Close failed connection
			db.Close()
			err = pingErr
		}

		// Check if we've exceeded max retries
		if maxRetries > 0 && attempt >= maxRetries {
			log.Printf("Failed to reconnect after %d attempts", attempt)
			return nil, err
		}

		log.Printf("Connection failed: %v (retry in %v)", err, delay)
		time.Sleep(delay)

		// Exponential backoff with cap at 60 seconds
		delay *= 2
		if delay > 60*time.Second {
			delay = 60 * time.Second
		}
	}
}

// EnsureConnection checks if the database connection is alive and reconnects if needed.
// This should be called periodically or before critical operations.
//
// Parameters:
//   - db: Current database connection
//   - cfg: Database configuration for reconnection
//
// Returns: Active database connection (either original or new) and error
func EnsureConnection(db *DB, cfg config.DatabaseConfig) (*DB, error) {
	// Check if connection is nil
	if db == nil {
		log.Println("Database connection is nil, attempting to reconnect...")
		return ReconnectWithRetry(cfg, 3, 1*time.Second)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Printf("Database connection lost: %v", err)
		log.Println("Attempting to reconnect...")

		// Close the old connection
		db.Close()

		// Attempt reconnection
		return ReconnectWithRetry(cfg, 3, 1*time.Second)
	}

	return db, nil
}

// HealthCheck performs a comprehensive health check on the database.
// Returns true if the database is healthy and ready for operations.
func HealthCheck(db *DB) bool {
	if db == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test basic connectivity
	if err := db.PingContext(ctx); err != nil {
		log.Printf("Health check failed - ping error: %v", err)
		return false
	}

	// Test a simple query
	var result int
	err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		log.Printf("Health check failed - query error: %v", err)
		return false
	}

	if result != 1 {
		log.Printf("Health check failed - unexpected result: %d", result)
		return false
	}

	return true
}

// WithRetry executes a database operation with automatic retry on connection failures.
// This provides transparent error recovery for transient database issues.
//
// Parameters:
//   - operation: Function to execute that may fail due to connection issues
//   - maxRetries: Maximum number of retry attempts
//
// Returns: Error from operation or nil on success
func WithRetry(operation func() error, maxRetries int) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := operation()

		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is related to connection
		// Common patterns: "connection refused", "broken pipe", "no connection"
		errStr := err.Error()
		isConnError := false
		connErrors := []string{
			"connection refused",
			"broken pipe",
			"no connection",
			"connection reset",
			"EOF",
			"timeout",
		}

		for _, pattern := range connErrors {
			if len(errStr) > 0 && contains(errStr, pattern) {
				isConnError = true
				break
			}
		}

		if !isConnError {
			// Not a connection error, don't retry
			return err
		}

		if attempt < maxRetries {
			waitTime := time.Duration(attempt+1) * time.Second
			log.Printf("Database operation failed (attempt %d/%d): %v (retry in %v)",
				attempt+1, maxRetries+1, err, waitTime)
			time.Sleep(waitTime)
		}
	}

	return lastErr
}

// contains checks if a string contains a substring (case-insensitive helper).
func contains(s, substr string) bool {
	// Simple case-insensitive check
	sLower := toLower(s)
	substrLower := toLower(substr)
	return len(sLower) >= len(substrLower) &&
		indexOfSubstring(sLower, substrLower) >= 0
}

// toLower converts string to lowercase.
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + ('a' - 'A')
		} else {
			result[i] = c
		}
	}
	return string(result)
}

// indexOfSubstring finds the index of substr in s, or -1 if not found.
func indexOfSubstring(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	if len(s) < len(substr) {
		return -1
	}

	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
