package adsb

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRetryWithBackoff tests basic retry logic.
func TestRetryWithBackoff(t *testing.T) {
	t.Run("Success on first attempt", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return nil
		}

		config := DefaultRetryConfig()
		err := RetryWithBackoff(context.Background(), config, operation)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if attempts != 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("Success after retries", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary error")
			}
			return nil
		}

		config := DefaultRetryConfig()
		err := RetryWithBackoff(context.Background(), config, operation)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("Max retries exceeded", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return errors.New("persistent error")
		}

		config := RetryConfig{
			MaxRetries:   3,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}
		err := RetryWithBackoff(context.Background(), config, operation)

		if err == nil {
			t.Error("Expected error after max retries")
		}
		// Should attempt: initial + 3 retries = 4 total
		if attempts != 4 {
			t.Errorf("Expected 4 attempts (initial + 3 retries), got %d", attempts)
		}
	})

	t.Run("Context cancellation", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return errors.New("error")
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		config := DefaultRetryConfig()
		err := RetryWithBackoff(ctx, config, operation)

		if err == nil {
			t.Error("Expected context cancellation error")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled error, got: %v", err)
		}
		// Should only attempt once when context is already canceled
		if attempts > 1 {
			t.Errorf("Expected 1 attempt, got %d", attempts)
		}
	})

	t.Run("Context timeout during retry", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return errors.New("error")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		config := RetryConfig{
			MaxRetries:   10,
			InitialDelay: 100 * time.Millisecond, // Longer than timeout
			MaxDelay:     1 * time.Second,
			Multiplier:   2.0,
		}

		start := time.Now()
		err := RetryWithBackoff(ctx, config, operation)
		elapsed := time.Since(start)

		if err == nil {
			t.Error("Expected timeout error")
		}
		// Should timeout within reasonable time
		if elapsed > 200*time.Millisecond {
			t.Errorf("Expected quick timeout, took %v", elapsed)
		}
	})

	t.Run("Exponential backoff calculation", func(t *testing.T) {
		attempts := 0
		delays := []time.Duration{}
		operation := func() error {
			attempts++
			if attempts < 4 {
				return errors.New("error")
			}
			return nil
		}

		config := RetryConfig{
			MaxRetries:   5,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}

		// Capture delays by measuring time between attempts
		lastTime := time.Now()
		wrappedOp := func() error {
			if attempts > 0 {
				delay := time.Since(lastTime)
				delays = append(delays, delay)
			}
			lastTime = time.Now()
			return operation()
		}

		err := RetryWithBackoff(context.Background(), config, wrappedOp)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// Verify exponential growth (with some tolerance for timing)
		if len(delays) >= 2 {
			// Second delay should be roughly 2x first delay
			// Allow 50% tolerance for timing variations
			ratio := float64(delays[1]) / float64(delays[0])
			if ratio < 1.5 || ratio > 2.5 {
				t.Errorf("Expected exponential backoff, delays: %v, ratio: %f", delays, ratio)
			}
		}
	})

	t.Run("Max delay cap", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			if attempts < 5 {
				return errors.New("error")
			}
			return nil
		}

		config := RetryConfig{
			MaxRetries:   10,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     20 * time.Millisecond, // Cap at 20ms
			Multiplier:   2.0,
		}

		start := time.Now()
		err := RetryWithBackoff(context.Background(), config, operation)
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}

		// With exponential backoff 2^n, without cap would be: 10, 20, 40, 80ms = 150ms
		// With 20ms cap: 10, 20, 20, 20ms = 70ms
		// Verify we're closer to capped time
		if elapsed > 120*time.Millisecond {
			t.Errorf("Expected max delay cap to limit total time, took %v", elapsed)
		}
	})
}

// TestRetryWithBackoffResult tests retry with result return.
func TestRetryWithBackoffResult(t *testing.T) {
	t.Run("Success with result", func(t *testing.T) {
		attempts := 0
		operation := func() (string, error) {
			attempts++
			if attempts < 2 {
				return "", errors.New("temporary error")
			}
			return "success", nil
		}

		config := DefaultRetryConfig()
		result, err := RetryWithBackoffResult(context.Background(), config, operation)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if result != "success" {
			t.Errorf("Expected result 'success', got %s", result)
		}
		if attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", attempts)
		}
	})

	t.Run("Failure returns zero value", func(t *testing.T) {
		operation := func() (int, error) {
			return 0, errors.New("persistent error")
		}

		config := RetryConfig{
			MaxRetries:   1,
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
		}
		result, err := RetryWithBackoffResult(context.Background(), config, operation)

		if err == nil {
			t.Error("Expected error")
		}
		if result != 0 {
			t.Errorf("Expected zero value (0), got %d", result)
		}
	})

	t.Run("Result type preserved", func(t *testing.T) {
		type customStruct struct {
			Value string
			Count int
		}

		operation := func() (customStruct, error) {
			return customStruct{Value: "test", Count: 42}, nil
		}

		config := DefaultRetryConfig()
		result, err := RetryWithBackoffResult(context.Background(), config, operation)

		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if result.Value != "test" || result.Count != 42 {
			t.Errorf("Expected custom struct with test/42, got %+v", result)
		}
	})
}

// TestDefaultRetryConfig tests default configuration.
func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", config.MaxRetries)
	}
	if config.InitialDelay != 1*time.Second {
		t.Errorf("Expected InitialDelay 1s, got %v", config.InitialDelay)
	}
	if config.MaxDelay != 60*time.Second {
		t.Errorf("Expected MaxDelay 60s, got %v", config.MaxDelay)
	}
	if config.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier 2.0, got %f", config.Multiplier)
	}
}

// TestZeroRetries tests behavior with no retries.
func TestZeroRetries(t *testing.T) {
	attempts := 0
	operation := func() error {
		attempts++
		return errors.New("error")
	}

	config := RetryConfig{
		MaxRetries:   0,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	err := RetryWithBackoff(context.Background(), config, operation)

	if err == nil {
		t.Error("Expected error")
	}
	// Should only attempt once (no retries)
	if attempts != 1 {
		t.Errorf("Expected 1 attempt with 0 retries, got %d", attempts)
	}
}

// TestRetryPreservesError tests that original error is returned.
func TestRetryPreservesError(t *testing.T) {
	expectedErr := errors.New("specific error message")
	operation := func() error {
		return expectedErr
	}

	config := RetryConfig{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	err := RetryWithBackoff(context.Background(), config, operation)

	if err == nil {
		t.Fatal("Expected error")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected error to be preserved, got: %v", err)
	}
}
