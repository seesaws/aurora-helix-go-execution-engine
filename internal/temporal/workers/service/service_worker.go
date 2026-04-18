package service

import (
	"github.com/aurora/aurora-helix-go-execution-engine/internal/temporal/workflows"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func Run(client client.Client, taskQueue string) error {
	w := worker.New(client, taskQueue, worker.Options{})
	w.RegisterWorkflow(workflows.ServiceDispatchWorkflow)
	return w.Run(worker.InterruptCh())
}
