package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server, err := testsuite.StartDevServer(ctx, testsuite.DevServerOptions{
		ClientOptions: &client.Options{HostPort: "127.0.0.1:7233", Namespace: "default"},
		LogLevel:      "warn",
	})
	if err != nil {
		panic(err)
	}
	defer server.Stop()

	fmt.Println(server.FrontendHostPort())
	<-ctx.Done()
}
