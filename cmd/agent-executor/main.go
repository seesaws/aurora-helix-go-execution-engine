package main

import (
	"log/slog"
	"os"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/config"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/executor/demo"
	temporalclient "github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/client"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/queues"
	agentworker "github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/workers/agent"
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

	if err := agentworker.Run(client, queues.AgentExecutorTaskQueue, demo.NewRegistry()); err != nil {
		logger.Error("agent executor stopped", "error", err)
		os.Exit(1)
	}
}
