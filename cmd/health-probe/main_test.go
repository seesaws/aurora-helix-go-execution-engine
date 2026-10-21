package main

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestTemporalProbeRejectsUnhealthyAndUnavailableServices(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(server, healthServer)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	const service = "temporal.api.workflowservice.v1.WorkflowService"
	healthServer.SetServingStatus(service, healthpb.HealthCheckResponse_NOT_SERVING)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := temporalHealth(ctx, listener.Addr().String()); err == nil {
		t.Fatal("unhealthy service was accepted")
	}
	healthServer.SetServingStatus(service, healthpb.HealthCheckResponse_SERVING)
	if err := temporalHealth(ctx, listener.Addr().String()); err != nil {
		t.Fatalf("healthy service rejected: %v", err)
	}
	server.Stop()
	short, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if err := temporalHealth(short, listener.Addr().String()); err == nil {
		t.Fatal("unavailable service was accepted")
	}
}
