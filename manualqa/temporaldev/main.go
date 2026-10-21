package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	hostPort := os.Getenv("TEMPORAL_HOST_PORT")
	if hostPort == "" {
		hostPort = "127.0.0.1:7233"
	}
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil || host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		panic("dev server must use a loopback host:port")
	}
	server, err := testsuite.StartDevServer(ctx, testsuite.DevServerOptions{
		ClientOptions: &client.Options{HostPort: hostPort, Namespace: "default"},
		LogLevel:      "warn",
	})
	if err != nil {
		panic(err)
	}
	defer server.Stop()

	fmt.Println(server.FrontendHostPort())
	<-ctx.Done()
}
