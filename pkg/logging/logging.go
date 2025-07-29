package logging

import (
	"context"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"
)

type Logger struct {
	*slog.Logger
}

type logLevel int

const (
	LevelDebug logLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

var (
	globalLogger     *Logger
	sensitiveEnvVars = []string{
		"PASSWORD", "PASS", "SECRET", "TOKEN", "KEY", "AUTH", "CREDENTIAL", "CRED",
		"GRAFANA_PASSWORD", "API_KEY", "PRIVATE_KEY", "CERT", "PEM",
	}
)

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

func Init() {
	logLevelStr := os.Getenv("LOG_LEVEL")
	if logLevelStr == "" {
		logLevelStr = "info"
	}

	logLevel := parseLogLevel(logLevelStr)

	opts := &slog.HandlerOptions{
		Level: logLevel.toSlogLevel(),
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
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

	globalLogger.Info("Logger initialized",
		"level", logLevelStr,
		"format", "logfmt",
	)

	if logLevel == LevelDebug {
		globalLogger.logEnvironmentVariables()
	}
}

func Get() *Logger {
	if globalLogger == nil {
		Init()
	}
	return globalLogger
}

func (l *Logger) logEnvironmentVariables() {
	envVars := make(map[string]string)

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		if l.isSensitiveEnvVar(key) {
			envVars[key] = "[REDACTED]"
		} else {
			envVars[key] = value
		}
	}

	l.Debug("Environment variables loaded", "env_vars", envVars)
}

func (l *Logger) isSensitiveEnvVar(varName string) bool {
	upperVarName := strings.ToUpper(varName)

	for _, sensitive := range sensitiveEnvVars {
		if strings.Contains(upperVarName, sensitive) {
			return true
		}
	}

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

func (l *Logger) WithContext(ctx context.Context) *Logger {
	return &Logger{Logger: l.Logger.With()}
}

func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{Logger: l.Logger.With(key, value)}
}

func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	args := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return &Logger{Logger: l.Logger.With(args...)}
}

func (l *Logger) Debug(msg string, args ...interface{}) {
	l.Logger.Debug(msg, args...)
}

func (l *Logger) Info(msg string, args ...interface{}) {
	l.Logger.Info(msg, args...)
}

func (l *Logger) Warn(msg string, args ...interface{}) {
	l.Logger.Warn(msg, args...)
}

func (l *Logger) Error(msg string, args ...interface{}) {
	l.Logger.Error(msg, args...)
}

func (l *Logger) DebugCall(functionName string, args ...interface{}) {
	l.Logger.Debug("Function call", append([]interface{}{"function", functionName}, args...)...)
}

func (l *Logger) DebugHTTP(method, url string, statusCode int, duration time.Duration, args ...interface{}) {
	baseArgs := []interface{}{
		"method", method,
		"url", url,
		"status_code", statusCode,
		"duration_ms", duration.Milliseconds(),
	}
	l.Logger.Debug("HTTP call", append(baseArgs, args...)...)
}

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
