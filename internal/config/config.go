package config

import "os"

type Config struct {
	HTTPPort          string
	TemporalHostPort  string
	TemporalNamespace string
}

func Load() Config {
	httpPort := os.Getenv("PORT")
	if httpPort == "" {
		httpPort = "8081"
	}
	temporalHostPort := os.Getenv("TEMPORAL_HOST_PORT")
	if temporalHostPort == "" {
		temporalHostPort = "localhost:7233"
	}
	temporalNamespace := os.Getenv("TEMPORAL_NAMESPACE")
	if temporalNamespace == "" {
		temporalNamespace = "default"
	}
	return Config{
		HTTPPort:          httpPort,
		TemporalHostPort:  temporalHostPort,
		TemporalNamespace: temporalNamespace,
	}
}
