package retry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxAttempts != 5 {
		t.Errorf("expected MaxAttempts=5, got %d", config.MaxAttempts)
	}
	if config.InitialDelay != 1*time.Second {
		t.Errorf("expected InitialDelay=1s, got %v", config.InitialDelay)
	}
	if config.MaxDelay != 32*time.Second {
		t.Errorf("expected MaxDelay=32s, got %v", config.MaxDelay)
	}
	if config.Multiplier != 2.0 {
		t.Errorf("expected Multiplier=2.0, got %f", config.Multiplier)
	}
	if !config.Jitter {
		t.Error("expected Jitter=true")
	}
}

func TestIsRetryable(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "rate limit error",
			err:      errors.New("Too many requests"),
			expected: true,
		},
		{
			name:     "429 error",
			err:      errors.New("HTTP 429: rate limited"),
			expected: true,
		},
		{
			name:     "throttling error",
			err:      errors.New("request throttled"),
			expected: true,
		},
		{
			name:     "service unavailable",
			err:      errors.New("503 ServiceUnavailable"),
			expected: true,
		},
		{
			name:     "non-retryable error",
			err:      errors.New("invalid credentials"),
			expected: false,
		},
		{
			name:     "not found error",
			err:      errors.New("resource not found"),
			expected: false,
		},
		{
			name:     "case insensitive match",
			err:      errors.New("TOO MANY REQUESTS"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.IsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestCalculateDelay(t *testing.T) {
	config := &Config{
		InitialDelay: 1 * time.Second,
		MaxDelay:     32 * time.Second,
		Multiplier:   2.0,
		Jitter:       false, // Disable jitter for predictable testing
	}

	tests := []struct {
		name     string
		attempt  int
		expected time.Duration
	}{
		{
			name:     "first retry (attempt 0)",
			attempt:  0,
			expected: 1 * time.Second, // 1 * 2^0 = 1
		},
		{
			name:     "second retry (attempt 1)",
			attempt:  1,
			expected: 2 * time.Second, // 1 * 2^1 = 2
		},
		{
			name:     "third retry (attempt 2)",
			attempt:  2,
			expected: 4 * time.Second, // 1 * 2^2 = 4
		},
		{
			name:     "fourth retry (attempt 3)",
			attempt:  3,
			expected: 8 * time.Second, // 1 * 2^3 = 8
		},
		{
			name:     "fifth retry (attempt 4)",
			attempt:  4,
			expected: 16 * time.Second, // 1 * 2^4 = 16
		},
		{
			name:     "sixth retry (attempt 5) - capped at max",
			attempt:  5,
			expected: 32 * time.Second, // 1 * 2^5 = 32 (capped)
		},
		{
			name:     "seventh retry (attempt 6) - still capped",
			attempt:  6,
			expected: 32 * time.Second, // Would be 64, but capped at 32
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.CalculateDelay(tt.attempt)
			if result != tt.expected {
				t.Errorf("CalculateDelay(%d) = %v, want %v", tt.attempt, result, tt.expected)
			}
		})
	}
}

func TestCalculateDelayWithJitter(t *testing.T) {
	config := &Config{
		InitialDelay: 1 * time.Second,
		MaxDelay:     32 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
	}

	// With jitter, delay should be within ±50% of base delay
	baseDelay := 2 * time.Second // For attempt 1
	minDelay := time.Duration(float64(baseDelay) * 0.5)
	maxDelay := time.Duration(float64(baseDelay) * 1.5)

	for i := 0; i < 100; i++ {
		delay := config.CalculateDelay(1)
		if delay < minDelay || delay > maxDelay {
			t.Errorf("CalculateDelay(1) with jitter = %v, want between %v and %v", delay, minDelay, maxDelay)
		}
	}
}

func TestDo_Success(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfig()
	config.MaxAttempts = 3

	attempts := 0
	err := Do(ctx, config, func() error {
		attempts++
		return nil
	})

	if err != nil {
		t.Errorf("Do() unexpected error: %v", err)
	}
	if attempts != 1 {
		t.Errorf("Do() attempts = %d, want 1", attempts)
	}
}

func TestDo_RetryableErrorThenSuccess(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		MaxAttempts:     3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		Multiplier:      2.0,
		Jitter:          false,
		RetryableErrors: []string{"Too many requests"},
	}

	attempts := 0
	err := Do(ctx, config, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("Too many requests")
		}
		return nil
	})

	if err != nil {
		t.Errorf("Do() unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("Do() attempts = %d, want 3", attempts)
	}
}

func TestDo_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfig()
	config.MaxAttempts = 3

	attempts := 0
	err := Do(ctx, config, func() error {
		attempts++
		return errors.New("invalid input")
	})

	if err == nil {
		t.Error("Do() expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("Do() attempts = %d, want 1 (should not retry non-retryable errors)", attempts)
	}
}

func TestDo_MaxAttemptsExceeded(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		MaxAttempts:     3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		Multiplier:      2.0,
		Jitter:          false,
		RetryableErrors: []string{"Too many requests"},
	}

	attempts := 0
	err := Do(ctx, config, func() error {
		attempts++
		return errors.New("Too many requests")
	})

	if err == nil {
		t.Error("Do() expected error, got nil")
	}
	if attempts != 3 {
		t.Errorf("Do() attempts = %d, want 3", attempts)
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "max retry attempts exceeded") {
		t.Errorf("Do() error should contain 'max retry attempts exceeded', got: %v", errMsg)
	}
}

func TestDo_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	config := &Config{
		MaxAttempts:     5,
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        1 * time.Second,
		Multiplier:      2.0,
		Jitter:          false,
		RetryableErrors: []string{"Too many requests"},
	}

	attempts := 0
	errChan := make(chan error)

	go func() {
		err := Do(ctx, config, func() error {
			attempts++
			return errors.New("Too many requests")
		})
		errChan <- err
	}()

	// Cancel after first attempt
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-errChan
	if err != context.Canceled {
		t.Errorf("Do() error = %v, want context.Canceled", err)
	}
}

func TestDoWithData_Success(t *testing.T) {
	ctx := context.Background()
	config := DefaultConfig()

	result, err := DoWithData(ctx, config, func() (string, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("DoWithData() unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("DoWithData() result = %q, want %q", result, "success")
	}
}

func TestDoWithData_RetryThenSuccess(t *testing.T) {
	ctx := context.Background()
	config := &Config{
		MaxAttempts:     3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		Multiplier:      2.0,
		Jitter:          false,
		RetryableErrors: []string{"Too many requests"},
	}

	attempts := 0
	result, err := DoWithData(ctx, config, func() (int, error) {
		attempts++
		if attempts < 2 {
			return 0, errors.New("Too many requests")
		}
		return 42, nil
	})

	if err != nil {
		t.Errorf("DoWithData() unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("DoWithData() result = %d, want 42", result)
	}
	if attempts != 2 {
		t.Errorf("DoWithData() attempts = %d, want 2", attempts)
	}
}
