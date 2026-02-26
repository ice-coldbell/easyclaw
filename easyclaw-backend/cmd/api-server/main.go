package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/coldbell/dex/backend/internal/apiserver"
	"github.com/coldbell/dex/backend/internal/config"
	"github.com/coldbell/dex/backend/internal/logging"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	bootstrapLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.LoadAPIServerConfig()
	if err != nil {
		bootstrapLogger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	logger, closeLogger, err := logging.New("api-server", cfg.Log)
	if err != nil {
		bootstrapLogger.Error("failed to initialize logger", "err", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := closeLogger(); closeErr != nil {
			bootstrapLogger.Error("failed to close logger", "err", closeErr)
		}
	}()

	if source, sourceErr := config.CurrentConfigSource(); sourceErr == nil {
		logger.Info("configuration loaded", "phase", source.Phase, "path", source.Path, "loaded", source.Loaded)
	}

	svc, err := apiserver.New(cfg, logger)
	if err != nil {
		logger.Error("failed to initialize api-server service", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := svc.Run(ctx); err != nil {
		logger.Error("api-server exited with error", "err", err)
		os.Exit(1)
	}
}
