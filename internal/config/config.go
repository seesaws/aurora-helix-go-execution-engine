package config

import "os"

type Config struct {
	ServiceToken      string
	SOPBaseURL        string
	HTTPHost          string
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
		ServiceToken:      os.Getenv("EXECUTION_SERVICE_TOKEN"),
		SOPBaseURL:        os.Getenv("SOP_BASE_URL"),
		HTTPHost:          os.Getenv("HTTP_HOST"),
		HTTPPort:          httpPort,
		TemporalHostPort:  temporalHostPort,
		TemporalNamespace: temporalNamespace,
	}
}
