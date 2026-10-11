package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// SetupLogging configures the process-wide slog logger to write text-format
// logs to a rotating file named log/<serviceName>.log, and returns a closer
// for that file.
//
// The log directory is created relative to the current working directory if it
// doesn't exist. The file rotates at 50 MB; at most 10 rotated files are kept,
// for at most 14 days, and rotated files are gzip-compressed. The level is Info
// unless the LOG_LEVEL environment variable is exactly "debug".
//
// Logs go only to the file, not to the terminal. Callers should defer the
// returned Closer's Close method so buffered output is released on shutdown.
// It returns an error if the log directory can't be created. serviceName is
// used in the filename as is, so it should not contain path separators.
func SetupLogging(serviceName string) (io.Closer, error) {
	const logDir = "log"

	// MkdirAll succeeds if the directory already exists.
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// lumberjack opens the file lazily on the first write and handles
	// rotation, so no file is created (and no error possible) until then.
	rotator := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, serviceName+".log"),
		MaxSize:    50,   // MB before rotating
		MaxBackups: 10,   // keep the last 10 rotated files
		MaxAge:     14,   // days
		Compress:   true, // gzip rotated files
	}

	// Default to Info; debug logging is opt-in through the environment.
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}

	// Replace the global logger so every slog.Info/Warn/Error call in the
	// program, including the ones in other packages, goes to the rotating file.
	slog.SetDefault(slog.New(slog.NewTextHandler(rotator, &slog.HandlerOptions{Level: level})))
	return rotator, nil
}
