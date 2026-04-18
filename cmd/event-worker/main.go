package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/app"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/config"
	temporalclient "github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/client"
	transporthttp "github.com/aurora/aurora-helix-go-execution-engine/internal/transport/http"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Load()
	tClient, err := temporalclient.New(cfg)
	if err != nil {
		logger.Error("failed to create temporal client", "error", err)
		os.Exit(1)
	}
	defer tClient.Close()

	dispatcher := app.NewTemporalDispatcher(tClient)
	server := transporthttp.NewServer(logger, dispatcher)
	httpServer := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting event worker", "port", cfg.HTTPPort)
		errCh <- httpServer.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("event worker stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}
}
