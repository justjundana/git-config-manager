// Package logger provides structured logging for GCM.
package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Level represents the severity of a log message.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Field is a key-value pair for structured logging.
type Field struct {
	Key   string
	Value interface{}
}

// F creates a new log field.
func F(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// Logger provides structured logging capabilities.
type Logger struct {
	mu      sync.Mutex
	level   Level
	output  io.Writer
	verbose bool
	quiet   bool
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Default returns the default logger instance.
func Default() *Logger {
	once.Do(func() {
		defaultLogger = New(LevelInfo, os.Stderr)
	})
	return defaultLogger
}

// New creates a new Logger.
func New(level Level, output io.Writer) *Logger {
	return &Logger{
		level:  level,
		output: output,
	}
}

// SetLevel sets the minimum log level.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetVerbose enables verbose (debug) output.
func (l *Logger) SetVerbose(verbose bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.verbose = verbose
	if verbose {
		l.level = LevelDebug
	}
}

// SetQuiet enables quiet mode (errors only).
func (l *Logger) SetQuiet(quiet bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.quiet = quiet
	if quiet {
		l.level = LevelError
	}
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...Field) {
	l.log(LevelDebug, msg, fields...)
}

// Info logs an informational message.
func (l *Logger) Info(msg string, fields ...Field) {
	l.log(LevelInfo, msg, fields...)
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...Field) {
	l.log(LevelWarn, msg, fields...)
}

// Error logs an error message.
func (l *Logger) Error(msg string, fields ...Field) {
	l.log(LevelError, msg, fields...)
}

// Fatal logs a fatal message and exits.
func (l *Logger) Fatal(msg string, fields ...Field) {
	l.log(LevelFatal, msg, fields...)
	os.Exit(1)
}

func (l *Logger) log(level Level, msg string, fields ...Field) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s %s", level, timestamp, msg)

	for _, f := range fields {
		entry += fmt.Sprintf(" %s=%v", f.Key, f.Value)
	}

	fmt.Fprintln(l.output, entry)
}

// Package-level convenience functions.

// Debug logs at debug level.
func Debug(msg string, fields ...Field) { Default().Debug(msg, fields...) }

// Info logs at info level.
func Info(msg string, fields ...Field) { Default().Info(msg, fields...) }

// Warn logs at warn level.
func Warn(msg string, fields ...Field) { Default().Warn(msg, fields...) }

// Error logs at error level.
func Error(msg string, fields ...Field) { Default().Error(msg, fields...) }

// Fatal logs at fatal level and exits.
func Fatal(msg string, fields ...Field) { Default().Fatal(msg, fields...) }
