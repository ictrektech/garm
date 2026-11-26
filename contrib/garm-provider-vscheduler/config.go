package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// config is the provider config file named by GARM_PROVIDER_CONFIG_FILE.
// JSON is used because this provider has no third party dependencies.
type config struct {
	// BaseURL is the V-Scheduler API root, for example
	// http://v-scheduler:38120/api/v1
	BaseURL string `json:"base_url"`
	// Token is the V-Scheduler service credential of the GARM application.
	Token string `json:"token"`
	// ContainerPort is the port inside the runner container. A runner does not
	// listen on a port, but V-Scheduler requires one and reserves a matching
	// host port from 42000-42999 on the target node.
	ContainerPort uint16 `json:"container_port"`
	// TimeoutSeconds bounds one V-Scheduler API call.
	TimeoutSeconds int `json:"timeout_seconds"`
	// ImageTag completes a pool image that has no tag. It is the part after the
	// architecture prefix, for example v6.0.1_3f9a1c2b. The provider prefixes it
	// with amd_ or arm_ from the pool os_arch, so one value serves both
	// architectures.
	ImageTag string `json:"image_tag"`
}

func loadConfig(path string) (config, error) {
	var cfg config
	if path == "" {
		return cfg, fmt.Errorf("GARM_PROVIDER_CONFIG_FILE is not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading provider config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing provider config %s: %w", path, err)
	}
	if cfg.BaseURL == "" {
		return cfg, fmt.Errorf("provider config %s: base_url is required", path)
	}
	if cfg.Token == "" {
		return cfg, fmt.Errorf("provider config %s: token is required", path)
	}
	if cfg.ContainerPort == 0 {
		cfg.ContainerPort = 8080
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 30
	}
	return cfg, nil
}

func (c config) timeout() time.Duration {
	return time.Duration(c.TimeoutSeconds) * time.Second
}
