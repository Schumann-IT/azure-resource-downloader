package retry

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"strings"
	"time"
)

// Config holds retry configuration
type Config struct {
	MaxAttempts     int           // Maximum number of retry attempts
	InitialDelay    time.Duration // Initial delay before first retry
	MaxDelay        time.Duration // Maximum delay between retries
	Multiplier      float64       // Backoff multiplier
	Jitter          bool          // Add random jitter to delays
	RetryableErrors []string      // Error messages that trigger retry
}

// DefaultConfig returns a sensible default retry configuration
func DefaultConfig() *Config {
	return &Config{
		MaxAttempts:  5,
		InitialDelay: 1 * time.Second,
		MaxDelay:     32 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		RetryableErrors: []string{
			"Too many requests",
			"TooManyRequests",
			"429",
			"rate limit",
			"RateLimitExceeded",
			"throttled",
			"Throttling",
			"ServiceUnavailable",
			"503",
			"RequestTimeout",
			"408",
		},
	}
}

// IsRetryable checks if an error should trigger a retry
func (c *Config) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	for _, retryableErr := range c.RetryableErrors {
		if strings.Contains(strings.ToLower(errMsg), strings.ToLower(retryableErr)) {
			return true
		}
	}

	return false
}

// CalculateDelay calculates the delay for a given attempt number
func (c *Config) CalculateDelay(attempt int) time.Duration {
	// Calculate exponential backoff: initialDelay * multiplier^attempt
	delay := float64(c.InitialDelay) * math.Pow(c.Multiplier, float64(attempt))

	// Cap at max delay
	if delay > float64(c.MaxDelay) {
		delay = float64(c.MaxDelay)
	}

	// Add jitter (random variation ±50% for better distribution)
	// This helps prevent "thundering herd" when many workers retry simultaneously
	if c.Jitter {
		jitter := delay * 0.5
		delay = delay - jitter + (rand.Float64() * 2 * jitter)
	}

	return time.Duration(delay)
}

// Do executes a function with retry logic
func Do(ctx context.Context, config *Config, fn func() error) error {
	if config == nil {
		config = DefaultConfig()
	}

	var lastErr error

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Execute the function
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !config.IsRetryable(err) {
			return err
		}

		// Last attempt, don't wait
		if attempt == config.MaxAttempts-1 {
			break
		}

		// Calculate delay and wait
		delay := config.CalculateDelay(attempt)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return errors.Join(lastErr, errors.New("max retry attempts exceeded"))
}

// DoWithData executes a function that returns data with retry logic
func DoWithData[T any](ctx context.Context, config *Config, fn func() (T, error)) (T, error) {
	if config == nil {
		config = DefaultConfig()
	}

	var lastErr error
	var result T

	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Execute the function
		data, err := fn()
		if err == nil {
			return data, nil
		}

		lastErr = err

		// Check if error is retryable
		if !config.IsRetryable(err) {
			return result, err
		}

		// Last attempt, don't wait
		if attempt == config.MaxAttempts-1 {
			break
		}

		// Calculate delay and wait
		delay := config.CalculateDelay(attempt)

		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return result, errors.Join(lastErr, errors.New("max retry attempts exceeded"))
}
