package adsb

import (
	"context"
	"fmt"
	"math"
	"time"
)

// RetryConfig configures retry behavior with exponential backoff.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (default: 3)
	MaxRetries int

	// InitialDelay is the initial backoff delay (default: 1 second)
	InitialDelay time.Duration

	// MaxDelay is the maximum backoff delay (default: 60 seconds)
	MaxDelay time.Duration

	// Multiplier is the backoff multiplier (default: 2.0 for exponential)
	Multiplier float64

	// RespectRetryAfter uses Retry-After header if available (default: true)
	RespectRetryAfter bool
}

// DefaultRetryConfig returns sensible defaults for retry behavior.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialDelay:      time.Second,
		MaxDelay:          60 * time.Second,
		Multiplier:        2.0,
		RespectRetryAfter: true,
	}
}

// RetryableFunc is a function that can be retried.
// It should return an error if the operation failed.
type RetryableFunc func() error

// RetryWithBackoff executes a function with exponential backoff retry logic.
// It handles rate limit errors (HTTP 429) specially by respecting Retry-After headers.
//
// Example usage:
//
//	err := RetryWithBackoff(ctx, DefaultRetryConfig(), func() error {
//	    aircraft, err := client.GetAircraft(lat, lon, radius)
//	    return err
//	})
func RetryWithBackoff(ctx context.Context, cfg RetryConfig, fn RetryableFunc) error {
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// First attempt (no delay)
		if attempt > 0 {
			// Check context before sleeping
			select {
			case <-ctx.Done():
				return fmt.Errorf("retry cancelled: %w", ctx.Err())
			case <-time.After(delay):
				// Continue with retry
			}
		}

		// Execute the function
		err := fn()
		if err == nil {
			return nil // Success!
		}

		lastErr = err

		// Check if it's a rate limit error
		if rle, ok := IsRateLimitError(err); ok {
			// If Retry-After header is present and we should respect it
			if cfg.RespectRetryAfter && rle.RetryAfter > 0 {
				delay = rle.RetryAfter
			}

			// Log rate limit information
			if rle.Headers.Remaining >= 0 {
				// Rate limit info available
				fmt.Printf("Rate limit hit: %d/%d requests remaining, reset at %v\n",
					rle.Headers.Remaining, rle.Headers.Limit, rle.Headers.Reset)
			}
		}

		// Last attempt - don't calculate next delay
		if attempt == cfg.MaxRetries {
			break
		}

		// Calculate next delay using exponential backoff
		// delay = min(InitialDelay * Multiplier^attempt, MaxDelay)
		nextDelay := time.Duration(float64(cfg.InitialDelay) * math.Pow(cfg.Multiplier, float64(attempt)))
		if nextDelay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		} else {
			delay = nextDelay
		}
	}

	return fmt.Errorf("max retries (%d) exceeded: %w", cfg.MaxRetries, lastErr)
}

// RetryWithBackoffResult executes a function with exponential backoff and returns a result.
// This is useful when the function returns data along with an error.
//
// Example usage:
//
//	aircraft, err := RetryWithBackoffResult(ctx, DefaultRetryConfig(), func() ([]Aircraft, error) {
//	    return client.GetAircraft(lat, lon, radius)
//	})
func RetryWithBackoffResult[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// First attempt (no delay)
		if attempt > 0 {
			// Check context before sleeping
			select {
			case <-ctx.Done():
				return result, fmt.Errorf("retry cancelled: %w", ctx.Err())
			case <-time.After(delay):
				// Continue with retry
			}
		}

		// Execute the function
		res, err := fn()
		if err == nil {
			return res, nil // Success!
		}

		result = res
		lastErr = err

		// Check if it's a rate limit error
		if rle, ok := IsRateLimitError(err); ok {
			// If Retry-After header is present and we should respect it
			if cfg.RespectRetryAfter && rle.RetryAfter > 0 {
				delay = rle.RetryAfter
			}

			// Log rate limit information
			if rle.Headers.Remaining >= 0 {
				// Rate limit info available
				fmt.Printf("Rate limit hit: %d/%d requests remaining, reset at %v\n",
					rle.Headers.Remaining, rle.Headers.Limit, rle.Headers.Reset)
			}
		}

		// Last attempt - don't calculate next delay
		if attempt == cfg.MaxRetries {
			break
		}

		// Calculate next delay using exponential backoff
		nextDelay := time.Duration(float64(cfg.InitialDelay) * math.Pow(cfg.Multiplier, float64(attempt)))
		if nextDelay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		} else {
			delay = nextDelay
		}
	}

	return result, fmt.Errorf("max retries (%d) exceeded: %w", cfg.MaxRetries, lastErr)
}
