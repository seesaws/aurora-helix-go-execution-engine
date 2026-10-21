package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/aurora/aurora-helix-go-execution-engine/internal/config"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/executor/system"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/activities"
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

	reporter, err := activities.NewCompletionReporter(cfg.SOPBaseURL, func(context.Context) (string, error) {
		token := os.Getenv("SOP_SERVICE_TOKEN")
		if token == "" {
			return "", fmt.Errorf("callback service credential unavailable")
		}
		return token, nil
	}, nil)
	if err != nil {
		logger.Error("invalid callback configuration")
		os.Exit(1)
	}
	if err := agentworker.Run(client, queues.AgentExecutorTaskQueue, system.Executor{}, reporter); err != nil {
		logger.Error("agent executor stopped", "error", err)
		os.Exit(1)
	}
}
