package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/coldbell/dex/backend/internal/config"
)

func New(serviceName string, cfg config.LogConfig) (*slog.Logger, func() error, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, nil, err
	}

	writer, closeWriter, err := openWriter(serviceName, cfg)
	if err != nil {
		return nil, nil, err
	}

	handlerOptions := &slog.HandlerOptions{Level: level}
	format := strings.ToLower(strings.TrimSpace(cfg.Format))
	if format == "" {
		format = "text"
	}

	var handler slog.Handler
	switch format {
	case "text":
		handler = slog.NewTextHandler(writer, handlerOptions)
	case "json":
		handler = slog.NewJSONHandler(writer, handlerOptions)
	default:
		_ = closeWriter()
		return nil, nil, fmt.Errorf("invalid log format %q (expected text|json)", cfg.Format)
	}

	logger := slog.New(handler).With("service", serviceName)
	return logger, closeWriter, nil
}

func openWriter(serviceName string, cfg config.LogConfig) (io.Writer, func() error, error) {
	output := strings.ToLower(strings.TrimSpace(cfg.Output))
	if output == "" {
		output = "console"
	}

	switch output {
	case "console":
		return os.Stdout, func() error { return nil }, nil
	case "file":
		file, err := openLogFile(serviceName, cfg.FilePath)
		if err != nil {
			return nil, nil, err
		}
		return file, file.Close, nil
	case "both":
		file, err := openLogFile(serviceName, cfg.FilePath)
		if err != nil {
			return nil, nil, err
		}
		multi := io.MultiWriter(os.Stdout, file)
		return multi, file.Close, nil
	default:
		return nil, nil, fmt.Errorf("invalid log output %q (expected console|file|both)", cfg.Output)
	}
}

func openLogFile(serviceName string, configuredPath string) (*os.File, error) {
	logPath := strings.TrimSpace(configuredPath)
	if logPath == "" {
		logPath = filepath.Join(".docker", serviceName, serviceName+".log")
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory for %q: %w", logPath, err)
	}

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", logPath, err)
	}
	return file, nil
}

func parseLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level %q (expected debug|info|warn|error)", raw)
	}
}
