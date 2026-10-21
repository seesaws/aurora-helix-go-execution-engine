package agent

import (
	"github.com/aurora/aurora-helix-go-execution-engine/internal/ports"
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/activities"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func Run(client client.Client, taskQueue string, registry ports.TaskExecutor, reporter *activities.CompletionReporter) error {
	w := worker.New(client, taskQueue, worker.Options{})
	executor := activities.NewAgentExecutor(registry)
	w.RegisterActivityWithOptions(executor.ExecuteTask, activity.RegisterOptions{Name: activities.ExecuteTaskActivityName})
	w.RegisterActivityWithOptions(reporter.DeliverResult, activity.RegisterOptions{Name: activities.DeliverResultActivityName})
	return w.Run(worker.InterruptCh())
}
