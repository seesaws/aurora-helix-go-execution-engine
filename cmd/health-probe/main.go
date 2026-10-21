// health-probe is a shell-free Kubernetes exec probe, not a worker HTTP server.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func temporalHealth(ctx context.Context, address string) error {
	conn, err := grpc.DialContext(ctx, address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return err
	}
	defer conn.Close()
	response, err := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{Service: "temporal.api.workflowservice.v1.WorkflowService"})
	if err != nil {
		return err
	}
	if response.Status != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("temporal is not serving")
	}
	return nil
}

func run(args []string) error {
	if len(args) == 2 && (args[0] == "process" || args[0] == "worker") {
		executable, err := os.Readlink("/proc/1/exe")
		if err != nil || filepath.Base(executable) != args[1] {
			return fmt.Errorf("expected worker is not PID 1")
		}
		if args[0] == "process" {
			return nil
		}
	} else if len(args) != 1 || args[0] != "temporal" {
		return fmt.Errorf("use process <binary>, worker <binary>, or temporal")
	}
	address := os.Getenv("TEMPORAL_HOST_PORT")
	if address == "" {
		address = "localhost:7233"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return temporalHealth(ctx, address)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		// Probe output is generic: never expose endpoints or transport credentials.
		fmt.Fprintln(os.Stderr, "health probe failed")
		os.Exit(1)
	}
}
