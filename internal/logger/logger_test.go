package logger

import (
	"os"
	"testing"

	"github.com/charmbracelet/log"
)

func TestNewLogger(t *testing.T) {
	logger := NewLogger()

	if logger == nil {
		t.Fatal("NewLogger() returned nil")
	}
}

func TestNewComponentLogger(t *testing.T) {
	componentLogger := NewComponentLogger("test-component")

	if componentLogger == nil {
		t.Fatal("NewComponentLogger() returned nil")
	}
}

func TestGetLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected log.Level
	}{
		{
			name:     "debug level",
			envValue: "debug",
			expected: log.DebugLevel,
		},
		{
			name:     "info level",
			envValue: "info",
			expected: log.InfoLevel,
		},
		{
			name:     "warn level",
			envValue: "warn",
			expected: log.WarnLevel,
		},
		{
			name:     "warning level",
			envValue: "warning",
			expected: log.WarnLevel,
		},
		{
			name:     "error level",
			envValue: "error",
			expected: log.ErrorLevel,
		},
		{
			name:     "fatal level",
			envValue: "fatal",
			expected: log.FatalLevel,
		},
		{
			name:     "default level (empty)",
			envValue: "",
			expected: log.InfoLevel,
		},
		{
			name:     "default level (invalid)",
			envValue: "invalid",
			expected: log.InfoLevel,
		},
		{
			name:     "case insensitive",
			envValue: "DEBUG",
			expected: log.DebugLevel,
		},
		{
			name:     "case insensitive mixed",
			envValue: "WaRn",
			expected: log.WarnLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envValue != "" {
				os.Setenv("LOG_LEVEL", tt.envValue)
			} else {
				os.Unsetenv("LOG_LEVEL")
			}
			defer os.Unsetenv("LOG_LEVEL")

			result := getLogLevel()
			if result != tt.expected {
				t.Errorf("getLogLevel() with LOG_LEVEL=%q = %v, want %v", tt.envValue, result, tt.expected)
			}
		})
	}
}

func TestDefaultLogger(t *testing.T) {
	if Default == nil {
		t.Fatal("Default logger is nil")
	}
}

func TestLoggerSetLevel(t *testing.T) {
	// Test that we can set different log levels
	logger := NewLogger()

	levels := []log.Level{
		log.DebugLevel,
		log.InfoLevel,
		log.WarnLevel,
		log.ErrorLevel,
	}

	for _, level := range levels {
		logger.SetLevel(level)
		currentLevel := logger.GetLevel()
		if currentLevel != level {
			t.Errorf("SetLevel(%v) failed, got level %v", level, currentLevel)
		}
	}
}
