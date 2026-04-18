package agent

import (
	"github.com/aurora/aurora-helix-go-execution-engine/internal/ports"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/activities"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func Run(client client.Client, taskQueue string, registry ports.TaskExecutor) error {
	w := worker.New(client, taskQueue, worker.Options{})
	executor := activities.NewAgentExecutor(registry)
	w.RegisterActivityWithOptions(executor.ExecuteTask, activity.RegisterOptions{Name: activities.ExecuteTaskActivityName})
	return w.Run(worker.InterruptCh())
}
