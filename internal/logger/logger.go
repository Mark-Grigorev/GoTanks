package logger

import (
	"fmt"
	"log/slog"
	"os"
)

// Logger wraps slog with printf-style methods.
type Logger struct {
	sl *slog.Logger
}

// New creates a Logger. debug=true → text handler at DEBUG level; false → JSON at INFO.
func New(debug bool) *Logger {
	var h slog.Handler
	if debug {
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	return &Logger{sl: slog.New(h)}
}

func (l *Logger) Debugf(format string, args ...any) {
	l.sl.Debug(fmt.Sprintf(format, args...))
}

func (l *Logger) Infof(format string, args ...any) {
	l.sl.Info(fmt.Sprintf(format, args...))
}

func (l *Logger) Warnf(format string, args ...any) {
	l.sl.Warn(fmt.Sprintf(format, args...))
}

func (l *Logger) Errorf(format string, args ...any) {
	l.sl.Error(fmt.Sprintf(format, args...))
}

// With returns a child Logger with additional structured key-value pairs.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{sl: l.sl.With(args...)}
}
