package main

import (
	"log/slog"
	"os"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/config"
	temporalclient "github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/client"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/queues"
	serviceworker "github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/workers/service"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Load()
	client, err := temporalclient.New(cfg)
	if err != nil {
		logger.Error("failed to create temporal client", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := serviceworker.Run(client, queues.ServiceWorkerTaskQueue); err != nil {
		logger.Error("service worker stopped", "error", err)
		os.Exit(1)
	}
}
