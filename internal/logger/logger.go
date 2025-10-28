package logger

import (
	"os"
	"strings"

	"github.com/charmbracelet/log"
)

var (
	// Default logger instance
	Default *log.Logger
)

func init() {
	Default = NewLogger()
}

// NewLogger creates a new logger with configuration from environment
func NewLogger() *log.Logger {
	opts := log.Options{
		ReportCaller:    false,
		ReportTimestamp: false, // CLI tools typically don't need timestamps
		Prefix:          "",
	}

	logger := log.NewWithOptions(os.Stderr, opts)

	// Set log level from environment
	level := getLogLevel()
	logger.SetLevel(level)

	return logger
}

// NewComponentLogger creates a logger for a specific component
func NewComponentLogger(component string) *log.Logger {
	return Default.With("component", component)
}

// getLogLevel reads LOG_LEVEL environment variable
func getLogLevel() log.Level {
	levelStr := strings.ToLower(os.Getenv("LOG_LEVEL"))

	switch levelStr {
	case "debug":
		return log.DebugLevel
	case "info":
		return log.InfoLevel
	case "warn", "warning":
		return log.WarnLevel
	case "error":
		return log.ErrorLevel
	case "fatal":
		return log.FatalLevel
	default:
		return log.InfoLevel // Default to info
	}
}
