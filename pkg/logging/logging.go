package logging

import (
	"context"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"
)

// Logger wraps the standard slog.Logger with additional functionality
type Logger struct {
	*slog.Logger
}

// logLevel represents the available log levels
type logLevel int

const (
	LevelDebug logLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

var (
	// Global logger instance
	globalLogger *Logger
	// Environment variable names that contain passwords or sensitive data
	sensitiveEnvVars = []string{
		"PASSWORD", "PASS", "SECRET", "TOKEN", "KEY", "AUTH", "CREDENTIAL", "CRED",
		"GRAFANA_PASSWORD", "API_KEY", "PRIVATE_KEY", "CERT", "PEM",
	}
)

// parseLogLevel converts string log level to logLevel enum
func parseLogLevel(level string) logLevel {
	switch strings.ToLower(level) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo // Default to info
	}
}

// toSlogLevel converts our logLevel to slog.Level
func (l logLevel) toSlogLevel() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Init initializes the global logger with the specified configuration
func Init() {
	// Get log level from environment variable, default to "info"
	logLevelStr := os.Getenv("LOG_LEVEL")
	if logLevelStr == "" {
		logLevelStr = "info"
	}

	logLevel := parseLogLevel(logLevelStr)

	// Create a text handler that outputs in logfmt format
	opts := &slog.HandlerOptions{
		Level: logLevel.toSlogLevel(),
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Format timestamp in a more readable way
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					return slog.Attr{
						Key:   a.Key,
						Value: slog.StringValue(t.Format("2006-01-02T15:04:05Z07:00")),
					}
				}
			}
			return a
		},
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	logger := slog.New(handler)

	globalLogger = &Logger{Logger: logger}

	// Log initialization message
	globalLogger.Info("Logger initialized",
		"level", logLevelStr,
		"format", "logfmt",
	)

	// Show environment variables in debug mode (excluding sensitive ones)
	if logLevel == LevelDebug {
		globalLogger.logEnvironmentVariables()
	}
}

// Get returns the global logger instance
func Get() *Logger {
	if globalLogger == nil {
		Init()
	}
	return globalLogger
}

// logEnvironmentVariables logs all environment variables except sensitive ones
func (l *Logger) logEnvironmentVariables() {
	envVars := make(map[string]string)

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		// Check if this environment variable contains sensitive data
		if l.isSensitiveEnvVar(key) {
			envVars[key] = "[REDACTED]"
		} else {
			envVars[key] = value
		}
	}

	l.Debug("Environment variables loaded", "env_vars", envVars)
}

// isSensitiveEnvVar checks if an environment variable name contains sensitive data
func (l *Logger) isSensitiveEnvVar(varName string) bool {
	upperVarName := strings.ToUpper(varName)

	for _, sensitive := range sensitiveEnvVars {
		if strings.Contains(upperVarName, sensitive) {
			return true
		}
	}

	// Additional regex patterns for common sensitive variable patterns
	patterns := []string{
		`.*_PASSWORD.*`,
		`.*_SECRET.*`,
		`.*_TOKEN.*`,
		`.*_KEY.*`,
		`.*_AUTH.*`,
		`.*_CREDENTIAL.*`,
		`.*_CRED.*`,
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, upperVarName); matched {
			return true
		}
	}

	return false
}

// WithContext returns a new logger with the given context
func (l *Logger) WithContext(ctx context.Context) *Logger {
	return &Logger{Logger: l.Logger.With()}
}

// WithField adds a field to the logger
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{Logger: l.Logger.With(key, value)}
}

// WithFields adds multiple fields to the logger
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	args := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return &Logger{Logger: l.Logger.With(args...)}
}

// Debug logs a debug message with optional key-value pairs
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.Logger.Debug(msg, args...)
}

// Info logs an info message with optional key-value pairs
func (l *Logger) Info(msg string, args ...interface{}) {
	l.Logger.Info(msg, args...)
}

// Warn logs a warning message with optional key-value pairs
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.Logger.Warn(msg, args...)
}

// Error logs an error message with optional key-value pairs
func (l *Logger) Error(msg string, args ...interface{}) {
	l.Logger.Error(msg, args...)
}

// DebugCall logs a debug message for function calls
func (l *Logger) DebugCall(functionName string, args ...interface{}) {
	l.Logger.Debug("Function call", append([]interface{}{"function", functionName}, args...)...)
}

// DebugHTTP logs HTTP request/response details
func (l *Logger) DebugHTTP(method, url string, statusCode int, duration time.Duration, args ...interface{}) {
	baseArgs := []interface{}{
		"method", method,
		"url", url,
		"status_code", statusCode,
		"duration_ms", duration.Milliseconds(),
	}
	l.Logger.Debug("HTTP call", append(baseArgs, args...)...)
}

// Package-level convenience functions
func Debug(msg string, args ...interface{}) {
	Get().Debug(msg, args...)
}

func Info(msg string, args ...interface{}) {
	Get().Info(msg, args...)
}

func Warn(msg string, args ...interface{}) {
	Get().Warn(msg, args...)
}

func Error(msg string, args ...interface{}) {
	Get().Error(msg, args...)
}

func DebugCall(functionName string, args ...interface{}) {
	Get().DebugCall(functionName, args...)
}

func DebugHTTP(method, url string, statusCode int, duration time.Duration, args ...interface{}) {
	Get().DebugHTTP(method, url, statusCode, duration, args...)
}

func WithField(key string, value interface{}) *Logger {
	return Get().WithField(key, value)
}

func WithFields(fields map[string]interface{}) *Logger {
	return Get().WithFields(fields)
}
