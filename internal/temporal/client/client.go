package client

import (
	"github.com/aurora/aurora-helix-go-execution-engine/internal/config"
	"go.temporal.io/sdk/client"
)

func New(cfg config.Config) (client.Client, error) {
	return client.Dial(client.Options{
		HostPort:  cfg.TemporalHostPort,
		Namespace: cfg.TemporalNamespace,
	})
}
